//go:build !cli

package gui

import (
	"crypto/rand"

	"app/core"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type LivePhotoReview struct {
	ID     string                `json:"id"`
	Groups []core.LivePhotoGroup `json:"groups"`
}

type LivePhotoReviewReply struct {
	ID    string               `json:"id"`
	Pairs []core.LivePhotoPair `json:"pairs"`
}

func (w *WailsReporter) ReviewLivePhotos(groups []core.LivePhotoGroup, cancel <-chan struct{}) []core.LivePhotoPair {
	id := rand.Text()
	answered := make(chan []core.LivePhotoPair, 1)
	off := w.app.Event.On("livePhotoReviewReply", func(e *application.CustomEvent) {
		reply, ok := e.Data.(LivePhotoReviewReply)
		if ok && reply.ID == id {
			select {
			case answered <- reply.Pairs:
			default:
			}
		}
	})
	defer off()
	w.app.Event.Emit("livePhotoReview", LivePhotoReview{ID: id, Groups: groups})
	select {
	case pairs := <-answered:
		return pairs
	case <-cancel:
		return nil
	}
}
