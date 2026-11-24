package core

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

// Small local files containing the same Apple Maker Note and QuickTime
// metadata that the desktop scanner uses to identify a pair.
func writeReviewFiles(t *testing.T, id, videoExt string) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	uint32s := func(numbers ...uint32) []byte {
		var data []byte
		for _, n := range numbers {
			data = binary.BigEndian.AppendUint32(data, n)
		}
		return data
	}
	box := func(kind string, parts ...[]byte) []byte {
		payload := bytes.Join(parts, nil)
		return bytes.Join([][]byte{uint32s(uint32(len(payload) + 8)), []byte(kind), payload}, nil)
	}
	note := bytes.Join([][]byte{
		[]byte("Apple iOS\x00\x00\x00MM\x00\x01\x00\x11\x00\x02"),
		uint32s(uint32(len(id)), 28), []byte(id),
	}, nil)
	still := write("IMG_0421.HEIC", note)
	variant := write("edited.JPG", append(note, []byte("edited pixels")...))
	keys := box("keys", uint32s(0, 1), box("mdta", []byte("com.apple.quicktime.content.identifier")))
	values := box("ilst", box("\x00\x00\x00\x01", box("data", uint32s(1, 0), []byte(id))))
	sample := box("mdat", uint32s(9, 1), []byte{0xff})
	table := box("stbl",
		box("stsd", uint32s(0, 1), box("mebx", []byte("com.apple.quicktime.still-image-time"))),
		box("stsz", uint32s(0, 9, 1)), box("stco", uint32s(0, 1, 8)))
	video := write("IMG_0421"+videoExt, bytes.Join([][]byte{sample, box("meta", keys, values), table}, nil))
	return still, variant, video
}

type reviewTestReporter struct {
	NopReporter
	results []FileUploadResult
}

func (r *reviewTestReporter) FileResult(result FileUploadResult) {
	r.results = append(r.results, result)
}

func TestHoldAndResolveLivePhotoGroups(t *testing.T) {
	const id = "54321abc-dead-beef-abcd-123456789012"
	const otherID = "12345abc-dead-beef-abcd-123456789012"
	still, variant, mov := writeReviewFiles(t, id, ".MOV")
	other, otherVariant, _ := writeReviewFiles(t, otherID, ".MOV")
	ordinary := filepath.Join(t.TempDir(), "ordinary.png")
	if err := os.WriteFile(ordinary, nil, 0600); err != nil {
		t.Fatal(err)
	}
	paths := []string{variant, ordinary, mov, still, other, otherVariant}
	opts := UploadOptions{PairLivePhotos: true}
	var held []LivePhotoGroup
	items, warnings, err := planUpload(paths, opts, nil, func(group LivePhotoGroup) { held = append(held, group) })
	if err != nil || len(warnings) != 0 || len(items) != 1 || items[0].Single.Path != ordinary {
		t.Fatalf("first transfers = %+v, warnings = %+v, %v", items, warnings, err)
	}
	want := []LivePhotoGroup{
		{ContentIdentifier: id, Photos: []string{variant, still}, Videos: []string{mov}},
		{ContentIdentifier: otherID, Photos: []string{other, otherVariant}, Videos: []string{}},
	}
	if !reflect.DeepEqual(held, want) {
		t.Fatalf("review groups = %+v", held)
	}
	cliItems, cliWarnings, err := PlanUpload(paths, opts, nil)
	if err != nil || len(cliItems) != len(paths) || len(cliWarnings) != 2 || cliWarnings[0].Code != "ambiguous-identifier" {
		t.Fatalf("noninteractive plan = %+v, %+v, %v", cliItems, cliWarnings, err)
	}

	reporter := &reviewTestReporter{}
	manager := NewUploadManager(reporter, nil)
	selection := LivePhotoPair{ContentIdentifier: id, PhotoPath: still, VideoPath: mov}
	items = manager.reviewedWorkItems(held, []LivePhotoPair{selection})
	if len(items) != 1 || items[0].LivePhoto == nil || *items[0].LivePhoto != selection {
		t.Fatalf("reviewed transfers = %+v", items)
	}
	if len(reporter.results) != 1 || !reporter.results[0].Skipped || !slices.Equal(reporter.results[0].Paths, []string{other, otherVariant}) {
		t.Fatalf("missing skipped group: %+v", reporter.results)
	}

	// A stale dialog, a missing companion or a file changed during a long batch
	// must never upload either component on its own.
	wrong := selection
	wrong.PhotoPath = other
	checkFailure := func(pair LivePhotoPair) {
		t.Helper()
		reporter.results = nil
		items := manager.reviewedWorkItems(held[:1], []LivePhotoPair{pair})
		if len(items) != 0 || len(reporter.results) != 1 || !reporter.results[0].IsError {
			t.Fatalf("invalid selection produced %+v, %+v", items, reporter.results)
		}
	}
	checkFailure(wrong)
	if err := os.WriteFile(still, []byte("replaced"), 0600); err != nil {
		t.Fatal(err)
	}
	checkFailure(selection)
	if err := os.Remove(mov); err != nil {
		t.Fatal(err)
	}
	selection.PhotoPath = variant
	checkFailure(selection)
}

func TestReviewIncludesMP4Companions(t *testing.T) {
	const id = "54321abc-dead-beef-abcd-123456789012"
	still, _, mov := writeReviewFiles(t, id, ".MOV")
	copyStill, _, mp4 := writeReviewFiles(t, id, ".Mp4")
	// A same-name ordinary MP4 beside a matching MOV is a separate upload.
	ordinary := filepath.Join(filepath.Dir(still), "IMG_0421.MP4")
	if err := os.WriteFile(ordinary, []byte("\x00\x00\x00\x0cftypisom"), 0600); err != nil {
		t.Fatal(err)
	}
	var groups []LivePhotoGroup
	items, warnings, err := planUpload([]string{still, mov, ordinary, copyStill, mp4}, UploadOptions{PairLivePhotos: true}, nil,
		func(group LivePhotoGroup) { groups = append(groups, group) })
	if err != nil || len(warnings) != 0 || len(items) != 1 || items[0].Single == nil || items[0].Single.Path != ordinary {
		t.Fatalf("ordinary uploads = %+v, warnings = %+v, error = %v", items, warnings, err)
	}
	want := []LivePhotoGroup{{ContentIdentifier: id, Photos: []string{still, copyStill}, Videos: []string{mov, mp4}}}
	if !reflect.DeepEqual(groups, want) {
		t.Fatalf("groups = %+v, want %+v", groups, want)
	}
	pair := LivePhotoPair{ContentIdentifier: id, PhotoPath: copyStill, VideoPath: mp4}
	items = NewUploadManager(&reviewTestReporter{}, nil).reviewedWorkItems(groups, []LivePhotoPair{pair})
	if len(items) != 1 || items[0].LivePhoto == nil || *items[0].LivePhoto != pair {
		t.Fatalf("reviewed MP4 pair = %+v", items)
	}

	// Even the filename override requires a motion timing track.
	items, warnings, err = PlanUpload([]string{still, ordinary}, UploadOptions{PairLivePhotos: true, IgnoreAppleMetadata: true}, nil)
	if err != nil || len(warnings) != 0 || len(items) != 2 || items[0].Single == nil || items[1].Single == nil {
		t.Fatalf("filename override = %+v, warnings = %+v, error = %v", items, warnings, err)
	}
}
