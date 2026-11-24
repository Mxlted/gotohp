package core

import (
	"path/filepath"
	"reflect"
	"testing"
)

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
	if legacy := (StartUploadEvent{}).UploadOptions(session); legacy != session || legacy.AlbumName != "Previous" {
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
