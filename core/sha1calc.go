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

	hash := sha1.New()
	cw := &chunkedContextWriter{ctx: ctx, w: hash}

	// Use a large buffer (1MB) to reduce syscall overhead
	buf := make([]byte, copyBufferSize)
	_, err = io.CopyBuffer(cw, file, buf)
	if err != nil {
		return nil, nil, fmt.Errorf("error calculating hash: %w", err)
	}

	return hash.Sum(nil), info, nil
}

type confirmedLocalFile struct {
	path string
	hash []byte
}

// Check both components of a Live Photo before removing either local copy.
func removeConfirmedFiles(ctx context.Context, files ...confirmedLocalFile) error {
	verified := make([]os.FileInfo, len(files))
	for i, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := os.Stat(file.path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is no longer a regular file", file.path)
		}
		hash, snapshot, err := fileSHA1(ctx, file.path)
		if err != nil {
			return err
		}
		if !bytes.Equal(hash, file.hash) {
			return fmt.Errorf("kept %s because its contents changed", file.path)
		}
		verified[i] = snapshot
	}
	for i, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		current, err := os.Stat(file.path)
		if err != nil {
			return err
		}
		before := verified[i]
		if !os.SameFile(before, current) || before.Size() != current.Size() || !before.ModTime().Equal(current.ModTime()) {
			return fmt.Errorf("kept %s because it changed during verification", file.path)
		}
		if err := os.Remove(file.path); err != nil {
			return err
		}
	}
	return nil
}
