package core

import (
	"errors"
	"slices"
)

func (g LivePhotoGroup) result() FileUploadResult {
	paths := slices.Concat(g.Photos, g.Videos)
	return FileUploadResult{IsLivePhoto: true, Path: paths[0], Paths: paths}
}

func (g LivePhotoGroup) resultWithError(err error) FileUploadResult {
	result := g.result()
	result.IsError, result.Error, result.ErrorMessage = true, err, err.Error()
	return result
}

func (m *UploadManager) reviewedWorkItems(groups []LivePhotoGroup, choices []LivePhotoPair) []UploadWorkItem {
	selected := make(map[string]LivePhotoPair, len(choices))
	for _, pair := range choices {
		selected[pair.ContentIdentifier] = pair
	}
	var items []UploadWorkItem
	for _, group := range groups {
		if m.isCancelled() {
			break
		}
		pair, ok := selected[group.ContentIdentifier]
		if !ok {
			result := group.result()
			result.Skipped, result.SkipCode = true, "live-photo-pair-not-selected"
			result.SkipReason = "No pair was selected for this Live Photo group."
			m.reporter.FileResult(result)
			continue
		}
		if !slices.Contains(group.Photos, pair.PhotoPath) || !slices.Contains(group.Videos, pair.VideoPath) {
			m.reporter.FileResult(group.resultWithError(errors.New("the chosen files do not belong to this Live Photo group")))
			continue
		}
		// The originals may have moved or changed while the rest of the batch ran.
		// Only send the exact pair the user reviewed, never a fallback single.
		work, _ := ClassifyUploadWork([]string{pair.PhotoPath, pair.VideoPath}, LivePhotoClassificationOptions{Enabled: true}, nil)
		if len(work) != 1 || work[0].LivePhoto == nil || *work[0].LivePhoto != pair {
			m.reporter.FileResult(group.resultWithError(errors.New("the selected still and video no longer form the original Live Photo pair. Check both files and try again")))
			continue
		}
		items = append(items, work[0])
	}
	return items
}
