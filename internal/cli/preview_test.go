package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"app/core"
)

func TestDryRunUsesLocalSelectionOnly(t *testing.T) {
	dir := uploadFilterFixture(t)
	config := filepath.Join(t.TempDir(), "unreadable-as-config.yml")
	if err := os.WriteFile(config, []byte("[invalid YAML"), 0o600); err != nil {
		t.Fatal(err)
	}
	rootFile := filepath.Join(dir, "root.jpg")
	nested := filepath.Join(dir, "nested", "child.jpg")
	root := newRootCommand(Info{ExecutableName: "gotohp-cli"})
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"upload", "--dry-run", "-r", "-e", "excluded", "--delete", "--album", "Trip",
		"--config", config, dir, rootFile})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var actual uploadPreview
	if err := json.Unmarshal(output.Bytes(), &actual); err != nil {
		t.Fatal(err)
	}
	want := uploadPreview{DryRun: true, Planned: 2, Items: []previewItem{
		{Kind: core.UploadWorkSingle, Paths: []string{nested}},
		{Kind: core.UploadWorkSingle, Paths: []string{rootFile}},
	}, Skipped: []uploadWarning{}, Warnings: []uploadWarning{}}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("preview = %+v, want %+v", actual, want)
	}
	for _, path := range []string{nested, rootFile} {
		if data, err := os.ReadFile(path); err != nil || string(data) != "filter fixture" {
			t.Fatalf("original changed: %s: %q, %v", path, data, err)
		}
	}
}

func TestDryRunReportsSkippedLivePhotoAndKeepsEmptyQueue(t *testing.T) {
	dir := t.TempDir()
	photo := filepath.Join(dir, "orphan.heic")
	if err := os.WriteFile(photo, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, "unused.config")
	root := newRootCommand(Info{ExecutableName: "gotohp-cli"})
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"upload", photo, "--dry-run", "--pair-live-photos", "--ignore-apple-metadata", "--config", config})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var actual uploadPreview
	if err := json.Unmarshal(output.Bytes(), &actual); err != nil {
		t.Fatal(err)
	}
	if !actual.DryRun || actual.Planned != 0 || actual.Items == nil || len(actual.Items) != 0 ||
		len(actual.Skipped) != 1 || actual.Skipped[0].Code != "incomplete-live-photo-skipped" ||
		!reflect.DeepEqual(actual.Skipped[0].Paths, []string{photo}) || len(actual.Warnings) != 0 {
		t.Fatalf("unexpected preview: %+v", actual)
	}
	if _, err := os.Stat(config); !os.IsNotExist(err) {
		t.Fatalf("dry run created config: %v", err)
	}
}
