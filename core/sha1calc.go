package core

import (
	"bytes"
	"context"
	"crypto/sha1"
	"fmt"
	"io"
	"os"
)

const (
	contextCheckInterval = 64 * 1024 * 1024 // Check every 64MB
	copyBufferSize       = 1 * 1024 * 1024  // 1MB copy buffer
)

type chunkedContextWriter struct {
	ctx             context.Context
	w               io.Writer
	bytesSinceCheck int64
}

func (cw *chunkedContextWriter) Write(p []byte) (int, error) {
	// Check context only after accumulating enough bytes
	if cw.bytesSinceCheck >= contextCheckInterval {
		select {
		case <-cw.ctx.Done():
			return 0, cw.ctx.Err()
		default:
			cw.bytesSinceCheck = 0
		}
	}

	n, err := cw.w.Write(p)
	cw.bytesSinceCheck += int64(n)
	return n, err
}

func CalculateSHA1(ctx context.Context, filePath string) ([]byte, error) {
	hash, _, err := fileSHA1(ctx, filePath)
	return hash, err
}

// fileSHA1 returns the identity of the opened file along with the hash of its contents.
func fileSHA1(ctx context.Context, filePath string) ([]byte, os.FileInfo, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("error opening file: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}

	hash, err := readerSHA1(ctx, file)
	if err != nil {
		return nil, nil, fmt.Errorf("error calculating hash: %w", err)
	}
	return hash, info, nil
}

func readerSHA1(ctx context.Context, reader io.Reader) ([]byte, error) {
	hash := sha1.New()
	cw := &chunkedContextWriter{ctx: ctx, w: hash}

	// Use a large buffer (1MB) to reduce syscall overhead
	buf := make([]byte, copyBufferSize)
	if _, err := io.CopyBuffer(cw, reader, buf); err != nil {
		return nil, err
	}
	return hash.Sum(nil), nil
}

type confirmedLocalFile struct {
	path string
	hash []byte
}

// Verify every component before making any changes to a local Live Photo.
func verifyConfirmedFiles(ctx context.Context, files []confirmedLocalFile) ([]os.FileInfo, error) {
	verified := make([]os.FileInfo, len(files))
	for i, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info, err := os.Stat(file.path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s is no longer a regular file", file.path)
		}
		hash, snapshot, err := fileSHA1(ctx, file.path)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(hash, file.hash) {
			return nil, fmt.Errorf("kept %s because its contents changed", file.path)
		}
		verified[i] = snapshot
	}
	return verified, nil
}

func checkFileSnapshot(path string, before os.FileInfo) error {
	current, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !sameFileSnapshot(before, current) {
		return fmt.Errorf("kept %s because it changed during verification", path)
	}
	return nil
}

func sameFileSnapshot(before, after os.FileInfo) bool {
	return os.SameFile(before, after) && before.Size() == after.Size() && before.ModTime().Equal(after.ModTime())
}

func removeConfirmedFiles(ctx context.Context, files ...confirmedLocalFile) error {
	verified, err := verifyConfirmedFiles(ctx, files)
	if err != nil {
		return err
	}
	for i, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := checkFileSnapshot(file.path, verified[i]); err != nil {
			return err
		}
		if err := os.Remove(file.path); err != nil {
			return err
		}
	}
	return nil
}
