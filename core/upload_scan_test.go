package core

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadScanIgnoresDeviceFiles(t *testing.T) {
	files, err := FilterGooglePhotosFiles([]string{os.DevNull}, UploadOptions{DisableUnsupportedFilesFilter: true})
	if err != nil || len(files) != 0 {
		t.Fatalf("device file selection = %v, %v", files, err)
	}
}

func TestUploadScanReportsBrokenMediaLink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "missing.jpg")
	if err := os.Symlink(filepath.Join(dir, "gone"), link); err != nil {
		t.Skipf("cannot create symlinks: %v", err)
	}
	files, err := FilterGooglePhotosFiles([]string{dir}, UploadOptions{})
	if !errors.Is(err, os.ErrNotExist) || len(files) != 0 {
		t.Fatalf("broken link selection = %v, %v", files, err)
	}
}

func TestRecursiveUploadScanReportsUnreadableDirectory(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "private")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(nested, 0); err != nil {
		t.Skipf("cannot set directory permissions: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(nested, 0o700) })
	if _, err := os.ReadDir(nested); !errors.Is(err, os.ErrPermission) {
		t.Skip("this filesystem or account can still read the directory")
	}
	files, err := FilterGooglePhotosFiles([]string{dir}, UploadOptions{Recursive: true})
	if !errors.Is(err, os.ErrPermission) || len(files) != 0 {
		t.Fatalf("unreadable directory selection = %v, %v", files, err)
	}
}
