package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	moveFolderPrefix = "Uploaded-"
	moveFolderMarker = ".gotohp-moved"
)

type uploadMoveBatch struct {
	destination   string
	folderName    string
	sourceFolders map[string]bool // Populated during the scan, before workers start.
	mu            sync.Mutex
	folders       map[string]string // One new folder per destination parent in this batch.
}

func newUploadMoveBatch(destination string, started time.Time) *uploadMoveBatch {
	return &uploadMoveBatch{
		destination:   destination,
		folderName:    moveFolderPrefix + started.Format("2006-01-02-150405.000"),
		sourceFolders: make(map[string]bool),
		folders:       make(map[string]string),
	}
}

func isUploadMoveFolder(path string) bool {
	if !strings.HasPrefix(filepath.Base(path), moveFolderPrefix) {
		return false
	}
	info, err := os.Lstat(filepath.Join(path, moveFolderMarker))
	return err == nil && info.Mode().IsRegular()
}

func (opts UploadOptions) finishLocalFiles(ctx context.Context, files []confirmedLocalFile) ([]string, error) {
	if len(files) == 0 {
		return nil, nil
	}
	if opts.MoveAfterUpload {
		batch := opts.moveBatch
		if batch == nil { // UploadFile can also be called directly for a single file.
			batch = newUploadMoveBatch(opts.MoveDestination, time.Now())
		}
		moved, err := batch.move(ctx, files)
		if err != nil {
			return moved, fmt.Errorf("Google Photos confirmed the media, but the local move failed: %w", err)
		}
		return moved, nil
	}
	if opts.DeleteFromHost {
		if err := removeConfirmedFiles(ctx, files...); err != nil {
			return nil, fmt.Errorf("Google Photos confirmed the media, but local deletion failed: %w", err)
		}
	}
	return nil, nil
}

func (b *uploadMoveBatch) target(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	parent, relative := filepath.Dir(absPath), filepath.Base(absPath)
	// Prefer the nearest selected folder when the queue contains overlapping selections.
	for dir := parent; ; dir = filepath.Dir(dir) {
		if b.sourceFolders[dir] {
			parent = filepath.Dir(dir)
			relative, err = filepath.Rel(dir, absPath)
			if err != nil {
				return "", err
			}
			break
		}
		if dir == filepath.Dir(dir) {
			break
		}
	}
	if b.destination != "" {
		parent, err = filepath.Abs(b.destination)
		if err != nil {
			return "", err
		}
	}
	folder, err := b.batchFolder(parent)
	if err != nil {
		return "", err
	}
	target := filepath.Join(folder, relative)
	return target, os.MkdirAll(filepath.Dir(target), 0o700)
}

func (b *uploadMoveBatch) batchFolder(parent string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if folder, ok := b.folders[parent]; ok {
		return folder, nil
	}
	for n := 1; ; n++ {
		name := b.folderName
		if n > 1 {
			name = fmt.Sprintf("%s-%d", name, n)
		}
		folder := filepath.Join(parent, name)
		if err := os.Mkdir(folder, 0o700); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return "", err
		}
		// A later recursive run can recognize its own output even when the user
		// put it inside a source tree. Other folders with similar names still scan.
		if err := os.WriteFile(filepath.Join(folder, moveFolderMarker), nil, 0o600); err != nil {
			_ = os.Remove(folder)
			return "", err
		}
		b.folders[parent] = folder
		return folder, nil
	}
}

type stagedMove struct {
	path string
	info os.FileInfo
}

func (file stagedMove) check() error {
	current, err := os.Lstat(file.path)
	if err != nil {
		return err
	}
	if !current.Mode().IsRegular() || !sameFileSnapshot(file.info, current) {
		return fmt.Errorf("the destination file %s changed during the move", file.path)
	}
	return nil
}

func (b *uploadMoveBatch) move(ctx context.Context, files []confirmedLocalFile) (moved []string, err error) {
	verified, err := verifyConfirmedFiles(ctx, files)
	if err != nil {
		return nil, err
	}
	// Secure every destination before removing either original of a Live Photo.
	var staged []stagedMove
	defer func() {
		if err != nil && len(moved) == 0 {
			for _, file := range staged {
				// Only roll back a destination that is still ours.
				if file.check() == nil {
					err = errors.Join(err, os.Remove(file.path))
				}
			}
		}
	}()
	for i, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		target, err := b.target(file.path)
		if err != nil {
			return nil, err
		}
		dest, err := stageMove(ctx, file, verified[i], target)
		if err != nil {
			return nil, err
		}
		staged = append(staged, dest)
	}
	for i, file := range files {
		if err := staged[i].check(); err != nil {
			return nil, err
		}
		if err := checkFileSnapshot(file.path, verified[i]); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for i, file := range files {
		if err := os.Remove(file.path); err != nil {
			// Once a source has been removed, retain the complete staged pair.
			if len(moved) > 0 {
				return moved, fmt.Errorf("saved %q, but could not remove its original: %w", staged[i].path, err)
			}
			return nil, err
		}
		moved = append(moved, staged[i].path)
	}
	return moved, nil
}

func stageMove(ctx context.Context, file confirmedLocalFile, info os.FileInfo, target string) (stagedMove, error) {
	entry, err := os.Lstat(file.path)
	if err != nil {
		return stagedMove{}, err
	}
	ext := filepath.Ext(target)
	for n := 1; ; n++ {
		if err := ctx.Err(); err != nil {
			return stagedMove{}, err
		}
		path := target
		if n > 1 {
			path = fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(target, ext), n, ext)
		}
		// A hard link preserves the original file and metadata without copying
		// large videos. Links and exclusive creates both refuse to replace a file.
		if entry.Mode().IsRegular() {
			err := os.Link(file.path, path)
			if err == nil {
				return stagedMove{path, info}, nil
			}
			if errors.Is(err, os.ErrExist) {
				continue
			}
		}
		// Other volumes and filesystems without hard links need a verified copy.
		// A selected symlink also takes this path so the destination holds real media.
		snapshot, err := copyConfirmedFile(ctx, file, info, path)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return stagedMove{path, snapshot}, err
	}
}

func copyConfirmedFile(ctx context.Context, file confirmedLocalFile, info os.FileInfo, target string) (snapshot os.FileInfo, err error) {
	in, err := os.Open(file.path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(target, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, out.Close())
		if err != nil {
			err = errors.Join(err, os.Remove(target))
		}
	}()
	if _, err := io.Copy(&chunkedContextWriter{ctx: ctx, w: out}, in); err != nil {
		return nil, err
	}
	if err := out.Sync(); err != nil {
		return nil, err
	}
	if _, err := out.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	hash, err := readerSHA1(ctx, out)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(hash, file.hash) {
		return nil, fmt.Errorf("the copy of %s did not match its verified contents", file.path)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.Chtimes(target, info.ModTime(), info.ModTime()); err != nil {
		return nil, err
	}
	if err := out.Chmod(info.Mode().Perm()); err != nil {
		return nil, err
	}
	return out.Stat()
}
