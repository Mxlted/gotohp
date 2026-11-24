export interface UploadSuccess {
  path: string
  mediaKey: string
}

export interface SkippedUploadResult {
  paths: string[]
  code: string
  reason: string
  mediaKey?: string
}

export interface UploadResults {
  success: UploadSuccess[]
  fail: string[]
  skipped: SkippedUploadResult[]
  warnings: SkippedUploadResult[]
}

function isIncompleteLivePhotoSkip(code: unknown): boolean {
  return code === 'incomplete-live-photo-skipped' || code === 'incomplete-live-photo-unavailable'
}

export function isSkippedUploadWarning(code: string): boolean {
  return isIncompleteLivePhotoSkip(code) || code === 'ambiguous-filename-stem'
}

// The desktop JSON report records preflight skips in "skipped", including in older versions.
export function incompleteLivePhotoPaths(report: unknown): string[] {
  if (!report || typeof report !== 'object' || !('skipped' in report) || !Array.isArray(report.skipped)) {
    throw new Error('Use Copy as JSON in the upload results to copy a report.')
  }
  const paths = new Set<string>()
  for (const item of report.skipped) {
    if (!isIncompleteLivePhotoSkip(item?.code) || !Array.isArray(item.paths)) continue
    for (const path of item.paths) {
      if (typeof path === 'string' && /\.(?:heic|heif|jpe?g|mov|mp4)$/i.test(path)) paths.add(path)
    }
  }
  return [...paths]
}

export interface UploadResultEvent {
  MediaKey: string
  IsError: boolean
  Skipped: boolean
  ErrorMessage: string
  SkipCode: string
  SkipReason: string
  Path: string
  Paths: string[]
}

export function recordUploadResult(results: UploadResults, event: UploadResultEvent): number {
  if (event.IsError) {
    results.fail.push([event.Path, event.ErrorMessage].filter(Boolean).join(': ') || 'Upload failed')
    return 1
  }

  if (event.Skipped) {
    const paths = event.Paths?.length ? event.Paths : [event.Path]
    results.skipped.push({
      paths: paths.filter(Boolean),
      code: event.SkipCode || 'skipped',
      reason: event.SkipReason || 'Skipped by upload policy',
      ...(event.MediaKey && { mediaKey: event.MediaKey }),
    })
    return 1
  }

  results.success.push({ path: event.Path, mediaKey: event.MediaKey })
  return 1
}
