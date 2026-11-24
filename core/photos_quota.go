package core

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Lookups, commits and album changes all consume the Photos service's
// 480-RPC per-minute allowance. Workers in the same batch share this clock.
const photosRPCInterval = time.Minute / 480

var errPhotosRateLimited = errors.New("Google Photos reached its request limit (HTTP 429)")

type photosRPCQuota struct {
	mu         sync.Mutex
	next       time.Time
	retryAt    time.Time
	cancel     <-chan struct{}
	onThrottle func(time.Duration)
}

func photosRetryTime(header string, now time.Time) time.Time {
	header = strings.TrimSpace(header)
	if seconds, err := strconv.ParseUint(header, 10, 32); err == nil {
		return now.Add(time.Duration(seconds) * time.Second)
	}
	if date, err := http.ParseTime(header); err == nil {
		return date
	}
	return now.Add(time.Minute)
}

func (q *photosRPCQuota) wait(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-q.cancel:
			return context.Canceled
		default:
		}
		q.mu.Lock()
		ready := q.next
		if q.retryAt.After(ready) {
			ready = q.retryAt
		}
		delay := time.Until(ready)
		if delay <= 0 {
			q.next = time.Now().Add(photosRPCInterval)
		}
		q.mu.Unlock()
		if delay <= 0 {
			return nil
		}
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-q.cancel:
			timer.Stop()
			return context.Canceled
		}
		// Another worker may have received a longer Retry-After while we slept.
	}
}

func (a *Api) doPhotosRPC(req *http.Request) (*http.Response, error) {
	if err := a.quota.wait(req.Context()); err != nil {
		return nil, err
	}
	resp, err := a.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusTooManyRequests {
		return resp, err
	}
	_ = resp.Body.Close()
	now := time.Now()
	retryAt := photosRetryTime(resp.Header.Get("Retry-After"), now)
	q := a.quota
	q.mu.Lock()
	notify := !q.retryAt.After(now) && retryAt.After(now)
	if retryAt.After(q.retryAt) {
		q.retryAt = retryAt
	}
	q.mu.Unlock()
	if notify && q.onThrottle != nil {
		q.onThrottle(retryAt.Sub(now))
	}
	return nil, errPhotosRateLimited
}
