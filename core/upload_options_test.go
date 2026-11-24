package core

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRetryOnlyAllowsSelectedIncompleteFiles(t *testing.T) {
	dir := t.TempDir()
	retry := filepath.Join(dir, "retry.heic")
	other := filepath.Join(dir, "other.heic")
	missing := filepath.Join(dir, "missing.mov")
	replaced := filepath.Join(dir, "replaced.heic")
	if err := os.Mkdir(replaced, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{retry, other, filepath.Join(replaced, "unrelated.jpg")} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	session := UploadOptions{PairLivePhotos: true, SkipIncompleteLivePhotos: true, IgnoreAppleMetadata: true}
	request := StartUploadEvent{
		Files:                []string{retry, other, missing, replaced},
		AllowIncompletePaths: []string{retry, missing, replaced},
	}
	run := request.UploadOptions(session)
	items, warnings, err := PlanUpload(request.Files, run, nil)
	if err != nil || len(items) != 1 || items[0].Kind != UploadWorkSingle || items[0].Single.Path != retry {
		t.Fatalf("retry plan = %+v, %v", items, err)
	}
	skipped := make(map[string]string)
	for _, warning := range warnings {
		if !IsSkippedPreflightWarning(warning.Code) || len(warning.Paths) != 1 {
			t.Fatalf("unexpected retry warning: %+v", warning)
		}
		skipped[warning.Paths[0]] = warning.Code
	}
	want := map[string]string{other: "incomplete-live-photo-skipped", missing: "incomplete-live-photo-unavailable", replaced: "incomplete-live-photo-unavailable"}
	if !reflect.DeepEqual(skipped, want) {
		t.Fatalf("skips = %v, want %v", skipped, want)
	}
	items, warnings, err = PlanUpload([]string{retry}, session, nil)
	if err != nil || len(items) != 0 || len(warnings) != 1 || warnings[0].Code != "incomplete-live-photo-skipped" || run.ForceUpload {
		t.Fatalf("normal run after retry = %+v, %+v, %v; retry force upload = %t", items, warnings, err, run.ForceUpload)
	}
}

type retryMetadata map[string]string

func (r retryMetadata) PhotoContentIdentifier(path string) (string, error) {
	return r[path], nil
}

func (r retryMetadata) VideoLivePhotoMetadata(path string) (LivePhotoMetadata, error) {
	return LivePhotoMetadata{ContentIdentifier: r[path], HasStillImageTime: true}, nil
}

func TestRetriedLivePhotoStillPairsWithAddedCompanion(t *testing.T) {
	paths := []string{"still.heic", "renamed.mov", "orphan.mov", "copy.heic", "edit.jpg", "copy.mov"}
	metadata := retryMetadata{
		"still.heic": "matched", "renamed.mov": "matched", "orphan.mov": "orphan",
		"copy.heic": "ambiguous", "edit.jpg": "ambiguous", "copy.mov": "ambiguous",
	}
	items, warnings := ClassifyUploadWork(paths, LivePhotoClassificationOptions{
		Enabled: true, SkipIncomplete: true, AllowIncompletePaths: map[string]bool{"still.heic": true, "orphan.mov": true},
	}, metadata)
	if len(warnings) != 1 || warnings[0].Code != "ambiguous-identifier" || IsSkippedPreflightWarning(warnings[0].Code) {
		t.Fatalf("warnings = %+v", warnings)
	}
	if len(items) != 5 || items[0].Kind != UploadWorkLivePhoto || !reflect.DeepEqual(items[0].Paths(), paths[:2]) {
		t.Fatalf("expected the recovered pair first, got %+v", items)
	}
	for index, path := range paths[2:] {
		item := items[index+1]
		if item.Kind != UploadWorkSingle || item.Single.Path != path {
			t.Fatalf("expected standalone %q, got %+v", path, item)
		}
	}
}

func TestReviewedDestinationAppliesOnlyToOneBatch(t *testing.T) {
	session := UploadOptions{
		Api: ApiOptions{Account: "previous@example.test"}, Recursive: true, AlbumName: "Previous",
	}
	request := StartUploadEvent{Destination: &UploadDestination{
		Account: "selected@example.test", AlbumName: " Holiday ",
	}}
	got := request.UploadOptions(session)
	if got.Api.Account != request.Destination.Account || got.AlbumName != "Holiday" || !got.Recursive || got.AlbumAutoMode {
		t.Fatalf("named album destination: %+v", got)
	}
	request.Destination = &UploadDestination{Account: "selected@example.test", AutoAlbum: true}
	got = request.UploadOptions(session)
	if !got.AlbumAutoMode || got.AlbumName != "" || got.Api.Account != request.Destination.Account {
		t.Fatalf("folder album destination: %+v", got)
	}
	if legacy := (StartUploadEvent{}).UploadOptions(session); !reflect.DeepEqual(legacy, session) || legacy.AlbumName != "Previous" {
		t.Fatalf("legacy session destination changed: %+v", legacy)
	}
}

func TestSessionUploadOptionsCapturePreferences(t *testing.T) {
	restoreConfigGlobals(t)
	if err := LoadConfig(filepath.Join(t.TempDir(), "gotohp.config")); err != nil {
		t.Fatal(err)
	}
	manager := &ConfigManager{}
	manager.SetProxy("http://proxy.example:8080")
	manager.SetSaver(true)
	manager.SetUseQuota(true)
	manager.SetUploadThreads(7)
	manager.SetPairLivePhotos(true)
	manager.SetSkipIncompleteLivePhotos(false)
	manager.SetUpdateExistingPhotosToLive(true)
	manager.SetAlbumName("Holiday")

	want := UploadOptions{
		Api:     ApiOptions{Proxy: "http://proxy.example:8080", Saver: true, UseQuota: true},
		Threads: 7, PairLivePhotos: true, UpdateExistingPhotosToLive: true,
		AlbumName: "Holiday",
	}
	captured := manager.SessionUploadOptions()
	if !reflect.DeepEqual(captured, want) {
		t.Fatalf("session options = %#v, want %#v", captured, want)
	}

	// Settings changed for the next run must not change an in-progress run.
	manager.SetProxy("")
	manager.SetSaver(false)
	manager.SetUseQuota(false)
	manager.SetUploadThreads(2)
	manager.SetAlbumName("")
	manager.SetAlbumAutoMode(true)
	if !reflect.DeepEqual(captured, want) {
		t.Fatalf("captured options changed after updating preferences: %#v", captured)
	}
	next := manager.SessionUploadOptions()
	if next.Api != (ApiOptions{}) || next.Threads != 2 || next.AlbumName != "" || !next.AlbumAutoMode {
		t.Fatalf("next run did not receive updated preferences: %#v", next)
	}
}
