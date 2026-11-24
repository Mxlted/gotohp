package core

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// FilesDroppedEvent is emitted when files are dropped on any drop zone
type FilesDroppedEvent struct {
	Files    []string `json:"files"`
	DropZone string   `json:"dropZone"`
}

// StartUploadEvent is received from frontend to start upload
type StartUploadEvent struct {
	Files                []string           `json:"files"`
	Destination          *UploadDestination `json:"destination,omitempty"`
	AllowIncompletePaths []string           `json:"allowIncompletePaths,omitempty"`
}

// UploadDestination is the account and album chosen for one reviewed desktop batch.
type UploadDestination struct {
	Account   string `json:"account"`
	AlbumName string `json:"albumName"`
	AutoAlbum bool   `json:"autoAlbum"`
}

// UploadOptions applies the reviewed destination and per-file retries to this run.
func (e StartUploadEvent) UploadOptions(opts UploadOptions) UploadOptions {
	if e.Destination != nil {
		opts.Api.Account = e.Destination.Account
		opts.AlbumName = strings.TrimSpace(e.Destination.AlbumName)
		opts.AlbumAutoMode = e.Destination.AutoAlbum
	}
	if len(e.AllowIncompletePaths) > 0 {
		opts.AllowIncompletePaths = make(map[string]bool, len(e.AllowIncompletePaths))
		for _, path := range e.AllowIncompletePaths {
			opts.AllowIncompletePaths[path] = true
		}
	}
	return opts
}

type UploadManager struct {
	mu       sync.Mutex
	wg       sync.WaitGroup
	cancel   chan struct{}
	canceled bool
	running  bool
	reporter UploadReporter
	logger   *slog.Logger
	// apiOptions is captured from the options of the run in progress.
	apiOptions ApiOptions
}

// NewUploadManager creates a manager that reports progress to reporter. A nil
// reporter or logger discards output.
func NewUploadManager(reporter UploadReporter, logger *slog.Logger) *UploadManager {
	if reporter == nil {
		reporter = NopReporter{}
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &UploadManager{
		reporter: reporter,
		logger:   logger,
	}
}

func (m *UploadManager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

func (m *UploadManager) Cancel() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil && !m.canceled {
		close(m.cancel)
		m.canceled = true
		// Don't set to nil - readers still need to detect closure via select
	}
}

// isCancelled checks if cancellation has been requested
func (m *UploadManager) isCancelled() bool {
	m.mu.Lock()
	cancel := m.cancel
	m.mu.Unlock()
	if cancel == nil {
		return false
	}
	select {
	case <-cancel:
		return true
	default:
		return false
	}
}

// getCancelChan returns the cancel channel safely
func (m *UploadManager) getCancelChan() <-chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cancel
}

type UploadBatchStart struct {
	Total      int   `json:"Total"`
	TotalBytes int64 `json:"TotalBytes"`
}

type FileUploadResult struct {
	MediaKey     string   `json:"MediaKey"`
	IsError      bool     `json:"IsError"`
	IsLivePhoto  bool     `json:"IsLivePhoto"`
	Skipped      bool     `json:"Skipped"`
	SkipCode     string   `json:"SkipCode"`
	SkipReason   string   `json:"SkipReason"`
	Error        error    `json:"-"`
	ErrorMessage string   `json:"ErrorMessage"`
	Path         string   `json:"Path"`
	Paths        []string `json:"Paths"`
}

type ThreadStatus struct {
	WorkerID      int    `json:"WorkerID"`
	Status        string `json:"Status"` // "idle", "hashing", "checking", "uploading", "finalizing", "completed", "skipped", "error"
	FilePath      string `json:"FilePath"`
	FileName      string `json:"FileName"`
	Message       string `json:"Message"`
	BytesUploaded int64  `json:"BytesUploaded"`
	BytesTotal    int64  `json:"BytesTotal"`
	Attempt       int    `json:"Attempt"` // Current attempt number (1-based), 0 if not applicable
}

// Upload starts an upload run in the background. Progress is delivered through
// the manager's UploadReporter; UploadStop marks the end of the run.
func (m *UploadManager) Upload(paths []string, opts UploadOptions) {
	opts = opts.normalized()
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.cancel = make(chan struct{})
	m.canceled = false
	m.apiOptions = opts.Api
	m.mu.Unlock()

	// Make preflight visible immediately so a long directory or metadata scan can
	// be cancelled from the UI.
	m.reporter.UploadStart(UploadBatchStart{})

	workItems, preflightWarnings, err := PlanUpload(paths, opts, m.isCancelled)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			m.finish()
			return
		}
		m.reporter.FileResult(FileUploadResult{
			IsError:      true,
			Error:        err,
			ErrorMessage: err.Error(),
		})
		m.finish()
		return
	}
	if len(workItems) == 0 && len(preflightWarnings) == 0 {
		preflightWarnings = append(preflightWarnings, PreflightWarning{
			Code:    "no-matching-files",
			Message: "No files matched the current file type and folder settings.",
		})
	}
	m.reportPreflight(len(workItems), preflightWarnings)

	if len(workItems) == 0 {
		m.finish()
		return
	}

	if _, err := NewApi(opts.Api); err != nil {
		for _, item := range workItems {
			path := uploadWorkPrimaryPath(item)
			m.reporter.FileResult(FileUploadResult{
				IsError:      true,
				Error:        err,
				ErrorMessage: err.Error(),
				Path:         path,
			})
		}
		m.finish()
		return
	}

	// Don't start more threads than files to process
	numWorkers := min(opts.Threads, len(workItems))

	// Create a worker pool for concurrent uploads
	workChan := make(chan UploadWorkItem, len(workItems))
	results := make(chan FileUploadResult, len(workItems))

	// Start workers
	for i := range numWorkers {
		m.wg.Add(1)
		go m.runWorker(i, workChan, results, opts)
	}

	// Send work to workers
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		var totalBytes int64
	LOOP:
		for _, item := range workItems {
			// Size each item before a worker can remove a confirmed local copy.
			for _, path := range item.Paths() {
				if info, err := os.Stat(path); err == nil {
					totalBytes += info.Size()
				}
			}
			select {
			case <-m.cancel:
				break LOOP
			case workChan <- item:
			}
		}
		m.reporter.TotalBytes(totalBytes)
		close(workChan)
	}()

	// Handle results, wait for completion, and create album if configured
	go func() {
		// Wait for all workers to finish in a separate goroutine, then close results
		go func() {
			m.wg.Wait()
			close(results)
		}()

		albumItems := m.collectResults(results)
		m.logger.Info("upload complete", "filesWithMediaKeys", len(albumItems),
			"albumName", opts.AlbumName, "albumAutoMode", opts.AlbumAutoMode)

		if len(albumItems) > 0 {
			m.handleAlbumCreation(albumItems, opts.AlbumName, opts.AlbumAutoMode)
		}

		m.finish()
	}()
}

func (m *UploadManager) collectResults(results <-chan FileUploadResult) map[string]string {
	albumItems := make(map[string]string)
	transferredPaths := make(map[string]string)
	for result := range results {
		m.reporter.FileResult(result)
		if result.IsError {
			m.logger.Error("upload error", "path", result.Path, "error", result.Error)
			continue
		}
		if result.MediaKey != "" {
			albumItems[result.Path] = result.MediaKey
		}
		if result.Skipped {
			m.logger.Info("upload skipped", "path", result.Path, "reason", result.SkipCode)
			continue
		}
		m.logger.Info("upload success", "path", result.Path)
		// Reconciliation intentionally targets an existing Live Photo. Only compare
		// receipts from ordinary transfers within this account and batch.
		if result.IsLivePhoto || result.MediaKey == "" {
			continue
		}
		if previous, found := transferredPaths[result.MediaKey]; found {
			m.reporter.Warning(PreflightWarning{
				Paths:   []string{previous, result.Path},
				Code:    "same-remote-item",
				Message: "Google Photos returned the same media ID for both transfers. These files share one library item.",
			})
		} else {
			transferredPaths[result.MediaKey] = result.Path
		}
	}
	return albumItems
}

// finish signals the end of a run and releases the manager for the next one.
func (m *UploadManager) finish() {
	m.reporter.UploadStop()
	m.mu.Lock()
	m.running = false
	m.mu.Unlock()
}

func (m *UploadManager) reportPreflight(uploadItemCount int, warnings []PreflightWarning) {
	skippedCount := 0
	for _, warning := range warnings {
		if IsSkippedPreflightWarning(warning.Code) {
			skippedCount++
		}
	}

	// Start before emitting warnings/results so the frontend resets its previous
	// batch while retaining every event from this preflight.
	m.reporter.UploadStart(UploadBatchStart{
		Total:      uploadItemCount + skippedCount,
		TotalBytes: 0,
	})
	for _, warning := range warnings {
		m.reporter.Warning(warning)
		if !IsSkippedPreflightWarning(warning.Code) {
			continue
		}
		primaryPath := ""
		if len(warning.Paths) > 0 {
			primaryPath = warning.Paths[0]
		}
		m.reporter.FileResult(FileUploadResult{
			IsLivePhoto: true,
			Skipped:     true,
			SkipCode:    warning.Code,
			SkipReason:  warning.Message,
			Path:        primaryPath,
			Paths:       warning.Paths,
		})
	}
}

// IsSkippedPreflightWarning reports whether the affected paths were left out of the plan.
func IsSkippedPreflightWarning(code string) bool {
	return code == "incomplete-live-photo-skipped" || code == "incomplete-live-photo-unavailable" || code == "ambiguous-filename-stem"
}

// handleAlbumCreation handles album creation based on config (manual name/key or AUTO mode)
func (m *UploadManager) handleAlbumCreation(uploads map[string]string, albumName string, albumAutoMode bool) {
	// Check if cancelled before starting album creation
	if m.isCancelled() {
		m.logger.Info("Upload cancelled, skipping album creation")
		return
	}

	m.logger.Info(fmt.Sprintf("handleAlbumCreation called with %d uploads", len(uploads)))

	// Create API once for all album operations
	api, err := NewApi(m.apiOptions)
	if err != nil {
		m.logger.Error(fmt.Sprintf("failed to create API for album creation: %v", err))
		m.reporter.AlbumError(AlbumError{
			AlbumName: albumName,
			Error:     fmt.Sprintf("failed to initialize API: %v", err),
		})
		return
	}

	albumManager := NewAlbumManager(api, m.reporter, m.logger, m.getCancelChan())

	// Check if AUTO mode is enabled
	if albumAutoMode {
		m.logger.Info("AUTO mode enabled, creating albums from directories")
		m.createAlbumsFromDirectories(albumManager, uploads)
		return
	}

	// Manual mode: use AlbumName if set
	if albumName == "" {
		m.logger.Info("No album name set and AUTO mode disabled, skipping album creation")
		return
	}

	m.logger.Info(fmt.Sprintf("Creating album with name/key: '%s'", albumName))

	mediaKeys := make([]string, 0, len(uploads))
	for _, mediaKey := range uploads {
		mediaKeys = append(mediaKeys, mediaKey)
	}

	m.logger.Info(fmt.Sprintf("Adding %d media keys to album '%s'", len(mediaKeys), albumName))

	albumKeys, err := albumManager.AddToAlbum(mediaKeys, albumName)
	if err != nil {
		m.logger.Error(fmt.Sprintf("failed to create album '%s': %v", albumName, err))
		m.reporter.AlbumError(AlbumError{
			AlbumName: albumName,
			Error:     err.Error(),
		})
		return
	}
	m.logger.Info(fmt.Sprintf("created album '%s' with %d items, album keys: %v", albumName, len(mediaKeys), albumKeys))
}

// createAlbumsFromDirectories creates albums based on parent directory names (AUTO mode)
func (m *UploadManager) createAlbumsFromDirectories(albumManager *AlbumManager, uploads map[string]string) {
	// Group media keys by parent directory
	mediaKeysByDir := make(map[string][]string)

	for filePath, mediaKey := range uploads {
		parentDir := filepath.Dir(filePath)
		mediaKeysByDir[parentDir] = append(mediaKeysByDir[parentDir], mediaKey)
	}

	// Create an album for each directory
	for dirPath, mediaKeys := range mediaKeysByDir {
		albumName := filepath.Base(dirPath)
		if albumName == "" || albumName == "." {
			albumName = "Uploads"
		}

		albumKeys, err := albumManager.AddToAlbum(mediaKeys, albumName)
		if err != nil {
			m.logger.Error(fmt.Sprintf("failed to create album '%s': %v", albumName, err))
			m.reporter.AlbumError(AlbumError{
				AlbumName: albumName,
				Error:     err.Error(),
			})
			continue
		}
		m.logger.Info(fmt.Sprintf("created album '%s' with %d items, album keys: %v", albumName, len(mediaKeys), albumKeys))
	}
}

// supportedFormats is a map of file extensions supported by Google Photos (O(1) lookup)
var supportedFormats = map[string]bool{
	// Photo formats
	"avif": true, "bmp": true, "gif": true, "heic": true, "heif": true, "ico": true,
	"jpg": true, "jpeg": true, "png": true, "tif": true, "tiff": true, "webp": true,
	"cr2": true, "cr3": true, "nef": true, "arw": true, "orf": true,
	"raf": true, "rw2": true, "pef": true, "sr2": true, "dng": true,
	// Video formats
	"3gp": true, "3g2": true, "asf": true, "avi": true, "divx": true,
	"m2t": true, "m2ts": true, "m4v": true, "mkv": true, "mmv": true,
	"mod": true, "mov": true, "mp4": true, "mpg": true, "mpeg": true,
	"mts": true, "tod": true, "wmv": true, "ts": true, "webm": true,
}

// isSupportedByGooglePhotos checks if a file extension is supported by Google Photos
func isSupportedByGooglePhotos(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		return false
	}
	// Remove the dot and check map
	return supportedFormats[ext[1:]]
}

func scanDirectoryForFiles(path string, recursive bool, excludePattern string, cancelled func() bool) ([]string, error) {
	var files []string

	if cancelled != nil && cancelled() {
		return nil, context.Canceled
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if cancelled != nil && cancelled() {
			return nil, context.Canceled
		}
		fullPath := filepath.Join(path, entry.Name())
		if entry.IsDir() {
			if excludePattern != "" && entry.Name() == excludePattern {
				continue
			}
			if recursive {
				subFiles, err := scanDirectoryForFiles(fullPath, recursive, excludePattern, cancelled)
				if err != nil {
					return nil, err
				}
				files = append(files, subFiles...)
			}
		} else {
			files = append(files, fullPath)
		}
	}

	return files, nil
}

// FilterGooglePhotosFiles expands paths into the files that would be uploaded
// under opts: directories are scanned, unsupported files dropped, duplicates removed.
func FilterGooglePhotosFiles(paths []string, opts UploadOptions) ([]string, error) {
	files, _, err := filterGooglePhotosFilesWithCancel(paths, opts, nil)
	return files, err
}

// PlanUpload expands the local selection and groups Live Photos in upload order.
// It reads local files only. Callers can supply a cancellation check for large selections.
func PlanUpload(paths []string, opts UploadOptions, cancelled func() bool) ([]UploadWorkItem, []PreflightWarning, error) {
	files, scanWarnings, err := filterGooglePhotosFilesWithCancel(paths, opts, cancelled)
	if err != nil {
		return nil, nil, err
	}
	items, warnings := ClassifyUploadWork(files, LivePhotoClassificationOptions{
		Enabled:              opts.PairLivePhotos,
		SkipIncomplete:       opts.SkipIncompleteLivePhotos,
		AllowIncompletePaths: opts.AllowIncompletePaths,
		IgnoreAppleMetadata:  opts.IgnoreAppleMetadata,
		Cancelled:            cancelled,
	}, nil)
	if cancelled != nil && cancelled() {
		return nil, nil, context.Canceled
	}
	return items, append(scanWarnings, warnings...), nil
}

func filterGooglePhotosFilesWithCancel(paths []string, opts UploadOptions, cancelled func() bool) ([]string, []PreflightWarning, error) {
	var supportedFiles []string
	var warnings []PreflightWarning
	type seenUploadFile struct {
		canonicalPath string
		info          os.FileInfo
	}
	seenFiles := make(map[string][]seenUploadFile)
	appendFile := func(path string, info os.FileInfo) error {
		if !opts.DisableUnsupportedFilesFilter && !isSupportedByGooglePhotos(path) {
			return nil
		}
		if info == nil {
			var err error
			info, err = os.Stat(path)
			if err != nil {
				return err
			}
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		canonicalPath := canonicalUploadPath(path)
		bucketKey := canonicalPath
		if runtime.GOOS == "windows" {
			bucketKey = strings.ToLower(bucketKey)
		}
		for _, seen := range seenFiles[bucketKey] {
			if canonicalPath == seen.canonicalPath || os.SameFile(info, seen.info) {
				return nil
			}
		}
		seenFiles[bucketKey] = append(seenFiles[bucketKey], seenUploadFile{
			canonicalPath: canonicalPath,
			info:          info,
		})
		supportedFiles = append(supportedFiles, path)
		return nil
	}

	for _, path := range paths {
		if cancelled != nil && cancelled() {
			return nil, nil, context.Canceled
		}
		fileInfo, err := os.Stat(path)
		if opts.AllowIncompletePaths[path] && (err != nil || !fileInfo.Mode().IsRegular()) {
			warnings = append(warnings, PreflightWarning{
				Paths:   []string{path},
				Code:    "incomplete-live-photo-unavailable",
				Message: "Could not retry this Live Photo. Restore the original file at this path and try again.",
			})
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("error accessing path %s: %w", path, err)
		}

		if fileInfo.IsDir() {
			files, err := scanDirectoryForFiles(path, opts.Recursive, opts.ExcludePattern, cancelled)
			if err != nil {
				return nil, nil, fmt.Errorf("error scanning directory %s: %w", path, err)
			}

			for _, file := range files {
				if cancelled != nil && cancelled() {
					return nil, nil, context.Canceled
				}
				if err := appendFile(file, nil); err != nil {
					return nil, nil, err
				}
			}
		} else if err := appendFile(path, fileInfo); err != nil {
			return nil, nil, err
		}
	}

	return supportedFiles, warnings, nil
}

func canonicalUploadPath(path string) string {
	canonicalPath, err := filepath.Abs(path)
	if err != nil {
		canonicalPath = filepath.Clean(path)
	}
	if resolved, err := filepath.EvalSymlinks(canonicalPath); err == nil {
		canonicalPath = resolved
	}
	return filepath.Clean(canonicalPath)
}

// UploadFile uploads a single file and returns its media key.
func UploadFile(ctx context.Context, api *Api, filePath string, opts UploadOptions, workerID int, reporter UploadReporter) (string, error) {
	if reporter == nil {
		reporter = NopReporter{}
	}
	key, _, err := uploadSingleFile(ctx, api, filePath, opts, workerID, reporter)
	return key, err
}

const remoteDuplicateReason = "Google Photos matched the file contents before upload."

func uploadSingleFile(ctx context.Context, api *Api, filePath string, opts UploadOptions, workerID int, reporter UploadReporter) (string, bool, error) {
	fileName := filepath.Base(filePath)

	// Stage 1: Hashing
	reporter.ThreadStatus(ThreadStatus{
		WorkerID: workerID,
		Status:   "hashing",
		FilePath: filePath,
		FileName: fileName,
		Message:  "Hashing...",
	})

	sha1Hash, fileInfo, err := fileSHA1(ctx, filePath)
	if err != nil {
		return "", false, fmt.Errorf("error calculating file hash: %w", err)
	}

	// Stage 2: Checking if exists in library
	if !opts.ForceUpload {
		reporter.ThreadStatus(ThreadStatus{
			WorkerID: workerID,
			Status:   "checking",
			FilePath: filePath,
			FileName: fileName,
			Message:  "Checking if file exists in library...",
		})

		mediaKey, err := api.FindRemoteMediaByHash(sha1Hash)
		if err != nil {
			reporter.Warning(PreflightWarning{
				Paths:   []string{filePath},
				Code:    "remote-check-failed",
				Message: fmt.Sprintf("The Google Photos duplicate check failed (%v). Continuing with the upload.", err),
			})
		} else if mediaKey != "" {
			reporter.TotalBytesDelta(-fileInfo.Size())
			reporter.ThreadStatus(ThreadStatus{
				WorkerID: workerID,
				Status:   "skipped",
				FilePath: filePath,
				FileName: fileName,
				Message:  remoteDuplicateReason,
			})
			if opts.DeleteFromHost {
				if err := removeConfirmedFiles(ctx, confirmedLocalFile{filePath, sha1Hash}); err != nil {
					return mediaKey, true, fmt.Errorf("file exists in library but failed to delete local copy: %w", err)
				}
			}
			return mediaKey, true, nil
		}
	}

	// Stage 3: Uploading
	fileSize := fileInfo.Size()
	reporter.ThreadStatus(ThreadStatus{
		WorkerID:      workerID,
		Status:        "uploading",
		FilePath:      filePath,
		FileName:      fileName,
		Message:       "Uploading...",
		BytesUploaded: 0,
		BytesTotal:    fileSize,
	})

	token, err := api.GetUploadToken(base64.StdEncoding.EncodeToString(sha1Hash), fileSize)
	if err != nil {
		return "", false, fmt.Errorf("error uploading file: %w", err)
	}

	// Create progress callback for upload
	progressCallback := func(bytesUploaded, bytesTotal int64, attempt int) {
		message := "Uploading..."
		if attempt > 1 {
			message = fmt.Sprintf("Retrying... (attempt %d)", attempt)
		}
		reporter.ThreadStatus(ThreadStatus{
			WorkerID:      workerID,
			Status:        "uploading",
			FilePath:      filePath,
			FileName:      fileName,
			Message:       message,
			BytesUploaded: bytesUploaded,
			BytesTotal:    bytesTotal,
			Attempt:       attempt,
		})
	}

	finalizeToken, err := api.UploadFileWithProgress(ctx, filePath, token, progressCallback)
	if err != nil {
		return "", false, fmt.Errorf("error uploading file: %w", err)
	}
	commitToken, err := finalizeToken.legacyCommitToken()
	if err != nil {
		return "", false, fmt.Errorf("error decoding upload finalize token: %w", err)
	}

	// Stage 4: Finalizing
	reporter.ThreadStatus(ThreadStatus{
		WorkerID: workerID,
		Status:   "finalizing",
		FilePath: filePath,
		FileName: fileName,
		Message:  "Committing upload...",
	})

	uploadTimestamp := fileInfo.ModTime().Unix()
	if opts.SetDateFromFilename {
		if t, ok := parseTimestampFromFilename(filePath); ok {
			uploadTimestamp = t.Unix()
		}
	}
	mediaKey, err := api.CommitUpload(commitToken, fileName, sha1Hash, uploadTimestamp)
	if err != nil {
		return "", false, fmt.Errorf("error committing file: %w", err)
	}

	if len(mediaKey) == 0 {
		return "", false, fmt.Errorf("media key not received")
	}

	if opts.DeleteFromHost {
		if err := removeConfirmedFiles(ctx, confirmedLocalFile{filePath, sha1Hash}); err != nil {
			return mediaKey, false, fmt.Errorf("uploaded successfully but failed to delete file: %w", err)
		}
	}

	return mediaKey, false, nil
}

func (m *UploadManager) runWorker(workerID int, workChan <-chan UploadWorkItem, results chan<- FileUploadResult, opts UploadOptions) {
	defer m.wg.Done()
	cancel := m.getCancelChan()

	// Emit idle status initially
	m.reporter.ThreadStatus(ThreadStatus{
		WorkerID: workerID,
		Status:   "idle",
		Message:  "Waiting for files...",
	})

	// Create API client once per worker for connection reuse
	api, err := NewApi(opts.Api)
	if err != nil {
		m.reporter.ThreadStatus(ThreadStatus{
			WorkerID: workerID,
			Status:   "error",
			Message:  fmt.Sprintf("Failed to initialize API: %v", err),
		})
		return
	}

	for item := range workChan {
		select {
		case <-cancel:
			m.reporter.ThreadStatus(ThreadStatus{
				WorkerID: workerID,
				Status:   "idle",
				Message:  "Cancelled",
			})
			return // Stop if cancellation is requested
		default:
			ctx, cancelUpload := context.WithCancel(context.Background())
			go func() {
				select {
				case <-cancel:
					cancelUpload()
				case <-ctx.Done():
					// Upload completed or context cancelled
				}
			}()

			results <- m.processWorkItem(ctx, api, item, opts, workerID)
			cancelUpload()

			// Mark as idle after completing file
			m.reporter.ThreadStatus(ThreadStatus{
				WorkerID: workerID,
				Status:   "idle",
				Message:  "Waiting for next file...",
			})
		}
	}

	// Final idle status when no more work
	m.reporter.ThreadStatus(ThreadStatus{
		WorkerID: workerID,
		Status:   "idle",
		Message:  "Finished",
	})
}

func (m *UploadManager) processWorkItem(ctx context.Context, api *Api, item UploadWorkItem, opts UploadOptions, workerID int) FileUploadResult {
	key, skipped, err := uploadWorkItem(ctx, api, item, opts, workerID, m.reporter)
	path := uploadWorkPrimaryPath(item)
	result := FileUploadResult{Path: path, Paths: item.Paths(), MediaKey: key, Skipped: skipped, IsLivePhoto: item.Kind == UploadWorkLivePhoto}
	status := ThreadStatus{WorkerID: workerID, FilePath: path, FileName: filepath.Base(path), Status: "completed", Message: "Completed"}
	if err != nil {
		if key == "" {
			result.IsError, result.Error, result.ErrorMessage = true, err, err.Error()
			status.Status, status.Message = "error", fmt.Sprintf("Error: %v", err)
			m.reporter.ThreadStatus(status)
			return result
		}
		// The library outcome is confirmed even when deleting the local copy fails.
		m.reporter.Warning(PreflightWarning{Paths: result.Paths, Code: "local-cleanup-failed", Message: err.Error()})
		status.Message = fmt.Sprintf("Uploaded, but local cleanup failed: %v", err)
	}
	if skipped {
		result.SkipCode, result.SkipReason = "remote-duplicate", remoteDuplicateReason
		if result.IsLivePhoto {
			result.SkipCode = "remote-live-photo-component-exists"
			result.SkipReason = "A component of this Live Photo already exists in Google Photos."
		}
		return result
	}
	m.reporter.ThreadStatus(status)
	return result
}

func uploadWorkItem(ctx context.Context, api *Api, item UploadWorkItem, opts UploadOptions, workerID int, reporter UploadReporter) (string, bool, error) {
	switch item.Kind {
	case UploadWorkSingle:
		if item.Single == nil || item.LivePhoto != nil {
			return "", false, fmt.Errorf("invalid single-media work item")
		}
		return uploadSingleFile(ctx, api, item.Single.Path, opts, workerID, reporter)
	case UploadWorkLivePhoto:
		if item.LivePhoto == nil || item.Single != nil {
			return "", false, fmt.Errorf("invalid Live Photo work item")
		}
		return uploadLivePhotoWithCallback(ctx, api, *item.LivePhoto, LivePhotoUploadOptions{
			Policy:                     buildLivePhotoCommitPolicy(api),
			DeleteFromHost:             opts.DeleteFromHost,
			SetDateFromFilename:        opts.SetDateFromFilename,
			UpdateExistingPhotosToLive: opts.UpdateExistingPhotosToLive,
		}, workerID, reporter)
	default:
		return "", false, fmt.Errorf("unsupported upload work kind %q", item.Kind)
	}
}

// Paths returns the local files in one work item, with the still image first for a Live Photo.
func (item UploadWorkItem) Paths() []string {
	if item.Kind == UploadWorkLivePhoto && item.LivePhoto != nil {
		return []string{item.LivePhoto.PhotoPath, item.LivePhoto.VideoPath}
	}
	if item.Single != nil {
		return []string{item.Single.Path}
	}
	return nil
}

func uploadWorkPrimaryPath(item UploadWorkItem) string {
	paths := item.Paths()
	if len(paths) == 0 {
		return ""
	}
	return paths[0]
}
