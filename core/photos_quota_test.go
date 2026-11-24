package core

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"google.golang.org/protobuf/proto"
)

func quotaResponse(header string) *http.Response {
	h := make(http.Header)
	if header != "" {
		h.Set("Retry-After", header)
	}
	return &http.Response{StatusCode: http.StatusTooManyRequests, Header: h, Body: io.NopCloser(strings.NewReader("\x08\x08\x12\x03raw"))}
}

func TestQuotaWaitAcrossWorkers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		header func(time.Time) string
		delay  time.Duration
	}{
		{"per-minute fallback", func(time.Time) string { return "" }, time.Minute},
		{"malformed header", func(time.Time) string { return "unknown" }, time.Minute},
		{"seconds", func(time.Time) string { return "75" }, 75 * time.Second},
		{"HTTP date", func(now time.Time) string { return now.Add(90 * time.Second).UTC().Format(http.TimeFormat) }, 90 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				match, err := proto.Marshal(duplicateMatch("original"))
				if err != nil {
					t.Fatal(err)
				}
				requests := make(chan time.Duration, 4)
				transport := func(*http.Request) (*http.Response, error) {
					elapsed := time.Since(start)
					requests <- elapsed
					if elapsed == 0 {
						return quotaResponse(tc.header(start)), nil
					}
					return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(match))}, nil
				}
				first, second := duplicateTestAPI(transport), duplicateTestAPI(transport)
				second.quota = first.quota
				check := func(api *Api) {
					key, err := api.checkUploadHash(context.Background(), []byte("hash"))
					if err != nil || key != "original" {
						t.Errorf("lookup = %q, %v", key, err)
					}
				}
				go check(first)
				synctest.Wait() // First worker received a 429 and is waiting to retry.
				go check(second)
				time.Sleep(tc.delay - time.Millisecond)
				synctest.Wait()
				if len(requests) != 1 {
					t.Fatalf("received %d requests during the shared cooldown", len(requests))
				}
				time.Sleep(time.Millisecond + photosRPCInterval)
				synctest.Wait()
				if len(requests) != 3 {
					t.Fatalf("received %d requests, want 3", len(requests))
				}
				firstCall, retry, nextCall := <-requests, <-requests, <-requests
				if firstCall != 0 || retry != tc.delay || nextCall-retry < photosRPCInterval {
					t.Fatalf("unexpected request times: %v, %v, %v", firstCall, retry, nextCall)
				}
			})
		})
	}
}

func TestPersistentOrCancelledQuotaKeepsLocalFile(t *testing.T) {
	for _, cancelCheck := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "still.jpg")
			if err := os.WriteFile(path, []byte("local original"), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			attempts := 0
			api := duplicateTestAPI(func(req *http.Request) (*http.Response, error) {
				if !strings.HasSuffix(req.URL.Path, "/5084965799730810217") {
					t.Fatalf("unchecked file reached %s", req.URL)
				}
				attempts++
				return quotaResponse(""), nil
			})
			if cancelCheck {
				time.AfterFunc(time.Second, cancel)
			}
			manager := NewUploadManager(nil, nil)
			result := manager.processWorkItem(ctx, api, UploadWorkItem{Kind: UploadWorkSingle, Single: &SingleMedia{Path: path}}, UploadOptions{DeleteFromHost: true}, 0)
			wantErr, wantAttempts := errPhotosRateLimited, DefaultRetryConfig().MaxRetries+1
			if cancelCheck {
				wantErr, wantAttempts = context.Canceled, 1
			}
			if !result.IsError || !errors.Is(result.Error, wantErr) || attempts != wantAttempts || strings.Contains(result.ErrorMessage, "raw") {
				t.Fatalf("unexpected outcome after %d attempts: %+v", attempts, result)
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != "local original" {
				t.Fatalf("local file changed: %q, %v", data, err)
			}
		})
	}
}
