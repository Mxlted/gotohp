package core

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
)

type livePhotoUploadAPI interface {
	checkUploadHash(ctx context.Context, hash []byte) (string, error)
	GetUploadToken(sha1Hash string, fileSize int64) (string, error)
	UploadFileWithProgress(ctx context.Context, filePath string, uploadToken string, onProgress UploadProgressCallback) (ScottyFinalizeToken, error)
	CommitLivePhoto(input LivePhotoCreateRequest) (string, error)
	ReconcileLivePhoto(input LivePhotoReconcileRequest) (string, error)
}

type LivePhotoUploadOptions struct {
	Policy                     LivePhotoCommitPolicy
	SetDateFromFilename        bool
	UpdateExistingPhotosToLive bool
}

func uploadLivePhotoWithCallback(
	ctx context.Context,
	api livePhotoUploadAPI,
	pair LivePhotoPair,
	options LivePhotoUploadOptions,
	workerID int,
	reporter UploadReporter,
) (uploadReceipt, error) {
	displayName := filepath.Base(pair.PhotoPath) + " + " + filepath.Base(pair.VideoPath)
	reporter.ThreadStatus(ThreadStatus{
		WorkerID: workerID,
		Status:   "hashing",
		FilePath: pair.PhotoPath,
		FileName: displayName,
		Message:  "Hashing Live Photo pair...",
	})

	photoSHA1, photoInfo, err := fileSHA1(ctx, pair.PhotoPath)
	if err != nil {
		return uploadReceipt{}, fmt.Errorf("hash Live Photo still: %w", err)
	}
	videoSHA1, videoInfo, err := fileSHA1(ctx, pair.VideoPath)
	if err != nil {
		return uploadReceipt{}, fmt.Errorf("hash Live Photo video: %w", err)
	}
	files := []confirmedLocalFile{{pair.PhotoPath, photoSHA1}, {pair.VideoPath, videoSHA1}}
	totalBytes := photoInfo.Size() + videoInfo.Size()

	reporter.ThreadStatus(ThreadStatus{
		WorkerID: workerID,
		Status:   "checking",
		FilePath: pair.PhotoPath,
		FileName: displayName,
		Message:  "Checking both Live Photo components...",
	})
	photoRemoteKey, err := api.checkUploadHash(ctx, photoSHA1)
	if err != nil {
		return uploadReceipt{}, fmt.Errorf("check Live Photo still deduplication: %w", err)
	}
	videoRemoteKey, err := api.checkUploadHash(ctx, videoSHA1)
	if err != nil {
		return uploadReceipt{}, fmt.Errorf("check Live Photo video deduplication: %w", err)
	}
	if photoRemoteKey != "" {
		if options.UpdateExistingPhotosToLive {
			reporter.TotalBytesDelta(-photoInfo.Size())
			mediaKey, err := reconcileExistingLivePhoto(
				ctx,
				api,
				pair,
				photoInfo,
				videoInfo,
				photoSHA1,
				videoSHA1,
				options,
				workerID,
				displayName,
				reporter,
			)
			if err != nil {
				return uploadReceipt{}, err
			}
			return uploadReceipt{mediaKey: mediaKey, files: files}, nil
		}
		reporter.TotalBytesDelta(-totalBytes)
		reporter.ThreadStatus(ThreadStatus{
			WorkerID: workerID,
			Status:   "skipped",
			FilePath: pair.PhotoPath,
			FileName: displayName,
			Message:  "Skipped: a Live Photo component already exists remotely",
		})
		return uploadReceipt{skipped: true}, nil
	}
	// The VideoOriginal direction has not been recovered. A standalone remote MOV
	// must not be combined with a newly uploaded still using the Phodeo request.
	if videoRemoteKey != "" {
		reporter.TotalBytesDelta(-totalBytes)
		reporter.ThreadStatus(ThreadStatus{
			WorkerID: workerID,
			Status:   "skipped",
			FilePath: pair.PhotoPath,
			FileName: displayName,
			Message:  "Skipped: the Live Photo video already exists remotely",
		})
		return uploadReceipt{skipped: true}, nil
	}

	photoToken, err := uploadLivePhotoComponent(ctx, api, pair.PhotoPath, pair.PhotoPath, photoInfo.Size(), photoSHA1, 0, totalBytes, workerID, displayName, reporter)
	if err != nil {
		return uploadReceipt{}, fmt.Errorf("upload Live Photo still: %w", err)
	}
	videoToken, err := uploadLivePhotoComponent(ctx, api, pair.VideoPath, pair.PhotoPath, videoInfo.Size(), videoSHA1, photoInfo.Size(), totalBytes, workerID, displayName, reporter)
	if err != nil {
		return uploadReceipt{}, fmt.Errorf("upload Live Photo video: %w", err)
	}

	uploadTime := photoInfo.ModTime()
	if options.SetDateFromFilename {
		if parsed, ok := parseTimestampFromFilename(pair.PhotoPath); ok {
			uploadTime = parsed
		}
	}
	reporter.ThreadStatus(ThreadStatus{
		WorkerID: workerID,
		Status:   "finalizing",
		FilePath: pair.PhotoPath,
		FileName: displayName,
		Message:  "Committing linked Live Photo...",
	})
	mediaKey, err := api.CommitLivePhoto(LivePhotoCreateRequest{
		PhotoToken:       photoToken,
		VideoToken:       videoToken,
		FileName:         photoInfo.Name(),
		PhotoSHA1:        photoSHA1,
		VideoSHA1:        videoSHA1,
		CreatedAt:        uploadTime,
		ModifiedAt:       uploadTime,
		StoragePolicy:    options.Policy.StoragePolicy,
		UploadQuality:    options.Policy.UploadQuality,
		UploadDeviceInfo: options.Policy.UploadDeviceInfo,
	})
	if err != nil {
		return uploadReceipt{}, fmt.Errorf("commit Live Photo: %w", err)
	}
	if mediaKey == "" {
		return uploadReceipt{}, fmt.Errorf("Live Photo media key not received")
	}
	return uploadReceipt{mediaKey: mediaKey, files: files}, nil
}

func reconcileExistingLivePhoto(
	ctx context.Context,
	api livePhotoUploadAPI,
	pair LivePhotoPair,
	photoInfo os.FileInfo,
	videoInfo os.FileInfo,
	photoSHA1 []byte,
	videoSHA1 []byte,
	options LivePhotoUploadOptions,
	workerID int,
	displayName string,
	reporter UploadReporter,
) (string, error) {
	reporter.ThreadStatus(ThreadStatus{
		WorkerID: workerID,
		Status:   "uploading",
		FilePath: pair.PhotoPath,
		FileName: displayName,
		Message:  "Uploading Live Photo video for existing photo...",
	})
	videoToken, err := uploadLivePhotoComponent(
		ctx,
		api,
		pair.VideoPath,
		pair.PhotoPath,
		videoInfo.Size(),
		videoSHA1,
		0,
		videoInfo.Size(),
		workerID,
		displayName,
		reporter,
	)
	if err != nil {
		return "", fmt.Errorf("upload Live Photo video for existing photo: %w", err)
	}

	uploadTime := photoInfo.ModTime()
	if options.SetDateFromFilename {
		if parsed, ok := parseTimestampFromFilename(pair.PhotoPath); ok {
			uploadTime = parsed
		}
	}
	reporter.ThreadStatus(ThreadStatus{
		WorkerID: workerID,
		Status:   "finalizing",
		FilePath: pair.PhotoPath,
		FileName: displayName,
		Message:  "Updating existing photo to Live...",
	})
	mediaKey, err := api.ReconcileLivePhoto(LivePhotoReconcileRequest{
		VideoToken:       videoToken,
		FileName:         videoInfo.Name(),
		PhotoSHA1:        photoSHA1,
		VideoSHA1:        videoSHA1,
		CreatedAt:        uploadTime,
		ModifiedAt:       uploadTime,
		StoragePolicy:    options.Policy.StoragePolicy,
		UploadQuality:    options.Policy.UploadQuality,
		UploadDeviceInfo: options.Policy.UploadDeviceInfo,
	})
	if err != nil {
		return "", fmt.Errorf("update existing photo to Live: %w", err)
	}
	if mediaKey == "" {
		return "", fmt.Errorf("updated Live Photo media key not received")
	}
	return mediaKey, nil
}

func uploadLivePhotoComponent(
	ctx context.Context,
	api livePhotoUploadAPI,
	componentPath string,
	progressPath string,
	size int64,
	hash []byte,
	completedBytes int64,
	totalBytes int64,
	workerID int,
	displayName string,
	reporter UploadReporter,
) (ScottyFinalizeToken, error) {
	uploadSession, err := api.GetUploadToken(base64.StdEncoding.EncodeToString(hash), size)
	if err != nil {
		return ScottyFinalizeToken{}, err
	}
	progress := func(bytesUploaded, _ int64, attempt int) {
		message := "Uploading Live Photo pair..."
		if attempt > 1 {
			message = fmt.Sprintf("Retrying Live Photo component (attempt %d)...", attempt)
		}
		reporter.ThreadStatus(ThreadStatus{
			WorkerID:      workerID,
			Status:        "uploading",
			FilePath:      progressPath,
			FileName:      displayName,
			Message:       message,
			BytesUploaded: completedBytes + bytesUploaded,
			BytesTotal:    totalBytes,
			Attempt:       attempt,
		})
	}
	return api.UploadFileWithProgress(ctx, componentPath, uploadSession, progress)
}
