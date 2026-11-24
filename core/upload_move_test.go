package core

import (
	"bytes"
	"context"
	"crypto/sha1"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"
)

func writeMoveFixture(t *testing.T, path, content string) confirmedLocalFile {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	hash := sha1.Sum([]byte(content))
	return confirmedLocalFile{path, hash[:]}
}

func TestMovePreservesSelectionLayout(t *testing.T) {
	for _, custom := range []bool{false, true} {
		name := "sibling"
		if custom {
			name = "custom parent inside source"
		}
		t.Run(name, func(t *testing.T) {
			completed := t.TempDir()
			root := filepath.Join(completed, "Google Photos")
			files := []confirmedLocalFile{
				writeMoveFixture(t, filepath.Join(root, "Photos from 2009", "photo.jpg"), "first"),
				writeMoveFixture(t, filepath.Join(root, "Photos from 2010", "Trip", "clip.mp4"), "second"),
				writeMoveFixture(t, filepath.Join(root, "cover.png"), "third"),
			}
			parent, destination := completed, ""
			if custom {
				parent = filepath.Join(root, "Archive")
				destination = parent
			}
			started := time.Date(2026, 11, 24, 0, 37, 27, 756000000, time.Local)
			batch := newUploadMoveBatch(destination, started)
			previous := writeMoveFixture(t, filepath.Join(parent, "Uploaded-2026-11-24-003727.756", "existing.txt"), "keep")
			opts := UploadOptions{Recursive: true, MoveAfterUpload: true, MoveDestination: destination, moveBatch: batch}
			// Even if a file was queued separately first, it belongs to its selected folder.
			items, _, err := PlanUpload([]string{files[0].path, root}, opts, nil)
			if err != nil || len(items) != len(files) {
				t.Fatalf("scan = %+v, %v", items, err)
			}
			var workers sync.WaitGroup
			for _, file := range files {
				workers.Go(func() {
					moved, err := opts.finishLocalFiles(t.Context(), []confirmedLocalFile{file})
					rel, _ := filepath.Rel(root, file.path)
					want := filepath.Join(parent, "Uploaded-2026-11-24-003727.756-2", rel)
					if err != nil || !slices.Equal(moved, []string{want}) {
						t.Errorf("move %s = %v, %v; want %s", file.path, moved, err, want)
						return
					}
					if hash, err := CalculateSHA1(t.Context(), want); err != nil || !bytes.Equal(hash, file.hash) {
						t.Errorf("destination differs from original: %s, %v", want, err)
					}
					if _, err := os.Stat(file.path); !os.IsNotExist(err) {
						t.Errorf("original still present: %s, %v", file.path, err)
					}
				})
			}
			workers.Wait()
			if got, err := os.ReadFile(previous.path); err != nil || string(got) != "keep" {
				t.Fatalf("pre-existing output changed: %q, %v", got, err)
			}
			next, err := FilterGooglePhotosFiles([]string{root}, opts)
			if err != nil || len(next) != 0 {
				t.Fatalf("rescan found already moved media: %v, %v", next, err)
			}
		})
	}
}

func TestMoveCollisionPreservesBothFiles(t *testing.T) {
	parent := t.TempDir()
	one := writeMoveFixture(t, filepath.Join(parent, "first", "year", "photo.jpg"), "one")
	two := writeMoveFixture(t, filepath.Join(parent, "second", "year", "photo.jpg"), "two")
	batch := newUploadMoveBatch("", time.Now())
	opts := UploadOptions{MoveAfterUpload: true, Recursive: true, moveBatch: batch}
	if _, _, err := PlanUpload([]string{filepath.Join(parent, "first"), filepath.Join(parent, "second")}, opts, nil); err != nil {
		t.Fatal(err)
	}
	for i, file := range []confirmedLocalFile{one, two} {
		moved, err := opts.finishLocalFiles(t.Context(), []confirmedLocalFile{file})
		name := []string{"photo.jpg", "photo (2).jpg"}[i]
		if want := filepath.Join(parent, batch.folderName, "year", name); err != nil || !slices.Equal(moved, []string{want}) {
			t.Fatalf("move = %q, %v; want %s", moved, err, want)
		}
	}
	for name, want := range map[string]string{"photo.jpg": "one", "photo (2).jpg": "two"} {
		if got, err := os.ReadFile(filepath.Join(parent, batch.folderName, "year", name)); err != nil || string(got) != want {
			t.Fatalf("%s = %q, %v; want %q", name, got, err, want)
		}
	}
}

func TestMovePairKeepsOriginalsOnFailure(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "Google Photos")
	photo := writeMoveFixture(t, filepath.Join(root, "still.heic"), "photo")
	video := writeMoveFixture(t, filepath.Join(root, "clips", "video.mov"), "video")
	files := []confirmedLocalFile{photo, video}
	batch := newUploadMoveBatch("", time.Now())
	batch.sourceFolders[root] = true
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := batch.move(cancelled, files); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled move = %v", err)
	}
	if _, err := batch.move(t.Context(), []confirmedLocalFile{photo, {video.path, photo.hash}}); err == nil {
		t.Fatal("move accepted an edited video")
	}
	if len(batch.folders) != 0 {
		t.Fatal("created a destination before verifying the pair")
	}
	folder, err := batch.batchFolder(parent)
	if err != nil {
		t.Fatal(err)
	}
	blocker := writeMoveFixture(t, filepath.Join(folder, "clips"), "do not overwrite")
	if moved, err := batch.move(t.Context(), files); err == nil || len(moved) != 0 {
		t.Fatalf("move with blocked video destination = %v, %v", moved, err)
	}
	for _, file := range []confirmedLocalFile{photo, video, blocker} {
		if got, err := CalculateSHA1(t.Context(), file.path); err != nil || !bytes.Equal(got, file.hash) {
			t.Fatalf("changed %s after failed move: %v", file.path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(folder, "still.heic")); !os.IsNotExist(err) {
		t.Fatalf("left a staged still: %v", err)
	}
}

func TestMoveCopyFallback(t *testing.T) {
	parent := t.TempDir()
	file := writeMoveFixture(t, filepath.Join(parent, "file.jpg"), "source")
	info, err := os.Stat(file.path)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(parent, "saved.jpg")
	if _, err := copyConfirmedFile(t.Context(), confirmedLocalFile{file.path, nil}, info, target); err == nil {
		t.Fatal("copied bytes without a matching hash")
	}
	snapshot, err := copyConfirmedFile(t.Context(), file, info, target)
	if err != nil {
		t.Fatal(err)
	}
	if err := (stagedMove{target, snapshot}).check(); err != nil {
		t.Fatal(err)
	}
	if os.SameFile(info, snapshot) || !info.ModTime().Equal(snapshot.ModTime()) {
		t.Fatalf("copy did not preserve the timestamp on a separate file: %v, %v", info, snapshot)
	}
	if _, err := copyConfirmedFile(t.Context(), file, info, target); !errors.Is(err, os.ErrExist) {
		t.Fatalf("copy accepted existing destination: %v", err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "source" {
		t.Fatalf("copy = %q, %v", got, err)
	}
}

func TestAfterUploadSettingsPersist(t *testing.T) {
	restoreConfigGlobals(t)
	parent := t.TempDir()
	config := filepath.Join(parent, "gotohp.config")
	writeMoveFixture(t, config, "preferences:\n  delete_from_host: true\n")
	if err := LoadConfig(config); err != nil {
		t.Fatal(err)
	}
	manager := &ConfigManager{}
	if opts := manager.SessionUploadOptions(); !opts.DeleteFromHost || opts.MoveAfterUpload {
		t.Fatalf("existing delete setting changed: %+v", opts)
	}
	if err := manager.SetAfterUploadAction("move", parent); err != nil {
		t.Fatal(err)
	}
	if err := LoadConfig(config); err != nil {
		t.Fatal(err)
	}
	opts := manager.SessionUploadOptions()
	if opts.DeleteFromHost || !opts.MoveAfterUpload || opts.MoveDestination != parent {
		t.Fatalf("reloaded move settings: %+v", opts)
	}
	if err := manager.SetAfterUploadAction("keep", parent); err != nil {
		t.Fatal(err)
	}
	if next := manager.SessionUploadOptions(); next.MoveAfterUpload || next.DeleteFromHost || !opts.MoveAfterUpload {
		t.Fatalf("changed previous batch or failed to keep next batch: before %+v, after %+v", opts, next)
	}
}
