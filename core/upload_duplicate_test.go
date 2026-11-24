package core

import (
	"bytes"
	"context"
	"crypto/sha1"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"app/generated"
	"google.golang.org/protobuf/proto"
)

type duplicateReporter struct {
	NopReporter
	warnings []PreflightWarning
	delta    int64
}

func (r *duplicateReporter) Warning(w PreflightWarning) { r.warnings = append(r.warnings, w) }
func (r *duplicateReporter) TotalBytesDelta(n int64)    { r.delta += n }

type duplicateTransport func(*http.Request) (*http.Response, error)

func (f duplicateTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func duplicateTestAPI(f duplicateTransport) *Api {
	return &Api{
		client:            &http.Client{Transport: f},
		quota:             &photosRPCQuota{},
		authResponseCache: map[string]string{"Expiry": "4102444800", "Auth": "test"},
	}
}

func duplicateMatch(key string) *generated.RemoteMatches {
	return &generated.RemoteMatches{Field1: &generated.RemoteMatchesField1Type{
		Field2: &generated.RemoteMatchesField1TypeField2Type{
			Field2: &generated.RemoteMatchesField1TypeField2TypeField2Type{MediaKey: key},
		},
	}}
}

func TestSingleFileDuplicateOutcomes(t *testing.T) {
	contents := []byte("identical file contents regardless of the name")
	hash := sha1.Sum(contents)
	for _, tc := range []struct {
		name              string
		match             bool
		force             bool
		lookupFails       bool
		cleanup           bool
		removeDuringCheck bool
		editDuringCheck   bool
		wantSkipped       bool
		wantWarning       string
	}{
		{name: "exact match", match: true, wantSkipped: true},
		{name: "clean up confirmed match", match: true, cleanup: true, wantSkipped: true},
		{name: "cleanup failure preserves match", match: true, cleanup: true, removeDuringCheck: true, wantSkipped: true, wantWarning: "local-cleanup-failed"},
		{name: "edit during lookup keeps newer copy", match: true, cleanup: true, editDuringCheck: true, wantSkipped: true, wantWarning: "local-cleanup-failed"},
		{name: "miss transfers the file"},
		{name: "clean up confirmed transfer", cleanup: true},
		{name: "force bypasses lookup", match: true, force: true},
		{name: "lookup failure is reported and upload continues", lookupFails: true, wantWarning: "remote-check-failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "renamed.jpg")
			if err := os.WriteFile(path, contents, 0600); err != nil {
				t.Fatal(err)
			}
			var calls []string
			api := duplicateTestAPI(func(req *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatal(err)
				}
				response := &http.Response{StatusCode: 200, Header: make(http.Header)}
				var reply proto.Message
				switch {
				case strings.HasSuffix(req.URL.Path, "/5084965799730810217"):
					calls = append(calls, "lookup")
					var query generated.HashCheck
					if err := proto.Unmarshal(body, &query); err != nil || !bytes.Equal(query.GetField1().GetField1().GetSha1Hash(), hash[:]) {
						t.Fatalf("lookup did not contain the file SHA-1: %v", err)
					}
					if tc.removeDuringCheck {
						if err := os.Remove(path); err != nil {
							t.Fatal(err)
						}
					}
					if tc.editDuringCheck {
						if err := os.WriteFile(path, bytes.Repeat([]byte("x"), len(contents)), 0600); err != nil {
							t.Fatal(err)
						}
					}
					if tc.lookupFails {
						response.StatusCode = http.StatusServiceUnavailable
					} else if tc.match {
						reply = duplicateMatch("test-media-key")
					}
				case req.Method == http.MethodPut:
					calls = append(calls, "transfer")
					if !bytes.Equal(body, contents) {
						t.Fatalf("unexpected media body %q", body)
					}
					reply = &generated.CommitToken{Field1: 2, Field2: []byte("test-receipt")}
				case strings.HasSuffix(req.URL.Path, "/uploadmedia/interactive"):
					calls = append(calls, "init")
					response.Header.Set("X-GUploader-UploadID", "test-session")
				case req.URL.String() == photosCreateMediaItemsEndpoint:
					calls = append(calls, "commit")
					reply = &generated.CreateMediaItemsResponse{Item: []*generated.CreateMediaItemResponseItem{{
						ResultItem: &generated.CreateMediaItemResult{MediaKey: "test-media-key"},
					}}}
				default:
					t.Fatalf("unexpected request %s %s", req.Method, req.URL)
				}
				data, err := proto.Marshal(reply)
				if err != nil {
					t.Fatal(err)
				}
				response.Body = io.NopCloser(bytes.NewReader(data))
				return response, nil
			})
			report := &duplicateReporter{}
			manager := NewUploadManager(report, nil)
			result := manager.processWorkItem(context.Background(), api, UploadWorkItem{
				Kind: UploadWorkSingle, Single: &SingleMedia{Path: path},
			}, UploadOptions{ForceUpload: tc.force, DeleteFromHost: tc.cleanup}, 0)
			if result.IsError || result.Skipped != tc.wantSkipped || result.MediaKey != "test-media-key" || result.Path != path {
				t.Fatalf("file result = %+v", result)
			}
			var expected []string
			if !tc.force {
				expected = append(expected, "lookup")
			}
			var delta int64
			if tc.wantSkipped {
				delta = -int64(len(contents))
				if result.SkipCode != "remote-duplicate" || result.SkipReason == "" {
					t.Fatalf("missing duplicate outcome: %+v", result)
				}
			} else {
				expected = append(expected, "init", "transfer", "commit")
			}
			if !slices.Equal(calls, expected) || report.delta != delta {
				t.Fatalf("requests = %v, byte adjustment = %d; want %v, %d", calls, report.delta, expected, delta)
			}
			if tc.wantWarning == "" {
				if len(report.warnings) != 0 {
					t.Fatalf("unexpected warnings = %+v", report.warnings)
				}
			} else if len(report.warnings) != 1 || report.warnings[0].Code != tc.wantWarning || !slices.Equal(report.warnings[0].Paths, []string{path}) {
				t.Fatalf("warnings = %+v, want %s for %s", report.warnings, tc.wantWarning, path)
			}
			wantRemoved := tc.cleanup && !tc.editDuringCheck
			if _, err := os.Stat(path); (wantRemoved && !os.IsNotExist(err)) || (!wantRemoved && err != nil) {
				t.Fatalf("unexpected local file state: %v", err)
			}
		})
	}
}

func TestLivePhotoComponentMatchIsAPartialSkip(t *testing.T) {
	dir := t.TempDir()
	pair := &LivePhotoPair{PhotoPath: filepath.Join(dir, "still.heic"), VideoPath: filepath.Join(dir, "motion.mov")}
	for _, path := range []string{pair.PhotoPath, pair.VideoPath} {
		if err := os.WriteFile(path, []byte(filepath.Base(path)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	lookups := 0
	api := duplicateTestAPI(func(req *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(req.URL.Path, "/5084965799730810217") {
			t.Fatalf("unexpected transfer for a partial match: %s", req.URL)
		}
		lookups++
		var body []byte
		if lookups == 1 {
			var err error
			body, err = proto.Marshal(duplicateMatch("still-only"))
			if err != nil {
				t.Fatal(err)
			}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body))}, nil
	})
	report := &duplicateReporter{}
	result := NewUploadManager(report, nil).processWorkItem(context.Background(), api, UploadWorkItem{
		Kind: UploadWorkLivePhoto, LivePhoto: pair,
	}, UploadOptions{}, 0)
	if lookups != 2 || result.IsError || !result.Skipped || result.SkipCode != "remote-live-photo-component-exists" || result.MediaKey != "" {
		t.Fatalf("component match was misclassified: %d lookups, result %+v", lookups, result)
	}
}

func TestLivePhotoCleanupChecksBothContentsBeforeRemoval(t *testing.T) {
	dir := t.TempDir()
	pair := LivePhotoPair{PhotoPath: filepath.Join(dir, "still.heic"), VideoPath: filepath.Join(dir, "video.mov")}
	photoHash, videoHash := sha1.Sum([]byte("still")), sha1.Sum([]byte("motion"))
	local := map[string]string{pair.PhotoPath: "newer", pair.VideoPath: "motion"}
	for path, contents := range local {
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := removeLivePhotoFiles(context.Background(), pair, photoHash[:], videoHash[:]); err == nil {
		t.Fatal("cleanup accepted an edited still")
	}
	for path, want := range local {
		if got, err := os.ReadFile(path); err != nil || string(got) != want {
			t.Fatalf("local pair changed: %s = %q, %v", path, got, err)
		}
	}
	if err := os.WriteFile(pair.PhotoPath, []byte("still"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := removeLivePhotoFiles(context.Background(), pair, photoHash[:], videoHash[:]); err != nil {
		t.Fatal(err)
	}
	for path := range local {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected confirmed copy %s to be removed, got %v", path, err)
		}
	}
}

func TestMatchedKeysAreAddedOncePerAlbum(t *testing.T) {
	sent := map[string][]string{}
	api := duplicateTestAPI(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		var add generated.AddMediaToAlbum
		if err := proto.Unmarshal(body, &add); err != nil {
			t.Fatal(err)
		}
		sent[add.AlbumMediaKey] = append(sent[add.AlbumMediaKey], add.MediaKeys...)
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	})
	manager := NewAlbumManager(api, NopReporter{}, nil, nil)
	for _, album := range []string{"AF1Qip-first", "AF1Qip-second"} {
		if _, err := manager.AddToAlbum([]string{"one", "two", "one"}, album); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(sent[album], []string{"one", "two"}) {
			t.Fatalf("keys sent to %s = %v", album, sent[album])
		}
	}
}

func TestShortAlbumKeyReturnsAlbumError(t *testing.T) {
	api := duplicateTestAPI(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody}, nil
	})
	manager := NewAlbumManager(api, NopReporter{}, nil, nil)
	if _, err := manager.AddToAlbum([]string{"matched-media"}, "AF1QipX"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected the server's album lookup error, got %v", err)
	}
}

func TestRepeatedCommitKeysOnlyCompareOrdinaryTransfers(t *testing.T) {
	report := &duplicateReporter{}
	manager := NewUploadManager(report, nil)
	items := []FileUploadResult{
		{Path: "present.jpg", Skipped: true, SkipCode: "remote-duplicate", MediaKey: "same"},
		{Path: "still.heic", IsLivePhoto: true, MediaKey: "same"},
		{Path: "first.jpg", MediaKey: "same"},
		{Path: "second.jpg", MediaKey: "same"},
		{Path: "another.jpg", MediaKey: "different"},
		{Path: "partial.heic", IsLivePhoto: true, Skipped: true},
		{Path: "failed.jpg", IsError: true},
	}
	ch := make(chan FileUploadResult, len(items))
	for _, item := range items {
		ch <- item
	}
	close(ch)
	album := manager.collectResults(ch)
	wantAlbum := map[string]string{"present.jpg": "same", "still.heic": "same", "first.jpg": "same", "second.jpg": "same", "another.jpg": "different"}
	if !reflect.DeepEqual(album, wantAlbum) {
		t.Errorf("album paths = %v, want %v", album, wantAlbum)
	}
	if len(report.warnings) != 1 || report.warnings[0].Code != "same-remote-item" || !slices.Equal(report.warnings[0].Paths, []string{"first.jpg", "second.jpg"}) {
		t.Fatalf("repeated-key warnings = %+v", report.warnings)
	}
}
