import type { LivePhotoGroup, LivePhotoPair } from '../../bindings/app/core/models'

export type FolderPriority = 'year' | 'album'
type PairPaths = Pick<LivePhotoPair, 'photoPath' | 'videoPath'>
type Candidates = Pick<LivePhotoGroup, 'photos' | 'videos'>
type Folder = Candidates & { rank: number, copy: number }

export function livePhotoDirectory(path: string) {
  const normalized = path.replaceAll('\\', '/')
  const end = normalized.lastIndexOf('/')
  if (end < 0) return ''
  const directory = normalized.slice(0, end) || '/'
  return /^(?:[a-z]:\/|\/\/)/i.test(normalized) ? directory.toLowerCase() : directory
}

export function livePhotoFolders(candidates: Candidates) {
  const folders = new Map<string, Candidates>()
  for (const part of ['photos', 'videos'] as const) {
    for (const path of candidates[part]) {
      const directory = livePhotoDirectory(path)
      let folder = folders.get(directory)
      if (!folder) {
        folder = { photos: [], videos: [] }
        folders.set(directory, folder)
      }
      folder[part].push(path)
    }
  }
  return folders
}

export function livePhotoFileName(path: string) {
  return path.slice(Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\')) + 1)
}

function folderRank(directory: string, first: FolderPriority) {
  const name = directory.slice(directory.lastIndexOf('/') + 1)
  const suffix = /\s*\((\d+)\)$/.exec(name)
  const original = suffix ? name.slice(0, suffix.index) : name
  const kind = /^(?:photos\s+from\s+)?\d{4}$/i.test(original) ? 'year' : 'album'
  let rank = kind === first ? 0 : 1
  if (suffix) rank += 2
  return { rank, copy: suffix ? Number(suffix[1]) : 0 }
}

function compareFolders(a: Folder, b: Folder) {
  return a.rank - b.rank || a.copy - b.copy
}

function filenameStem(path: string) {
  const name = livePhotoFileName(path)
  return name.slice(0, name.lastIndexOf('.')).toLowerCase()
}

function matchFilenames({ photos, videos }: Candidates): PairPaths | undefined {
  const byStem = new Map<string, string>()
  for (const video of videos) {
    const stem = filenameStem(video)
    if (!byStem.has(stem)) byStem.set(stem, video)
  }
  for (const photoPath of photos) {
    const videoPath = byStem.get(filenameStem(photoPath))
    if (videoPath) return { photoPath, videoPath }
  }
}

// All candidates already share an Apple content identifier. Folder and file
// names only help choose between those variants.
export function suggestLivePhotoPair(group: Candidates, first: FolderPriority, manual: Partial<PairPaths> = {}): PairPaths | undefined {
  const candidates = {
    photos: manual.photoPath ? group.photos.filter(path => path === manual.photoPath) : group.photos,
    videos: manual.videoPath ? group.videos.filter(path => path === manual.videoPath) : group.videos,
  }
  if (!candidates.photos.length || !candidates.videos.length) return

  const ranked = Array.from(livePhotoFolders(candidates), ([directory, files]) => ({
    ...files, ...folderRank(directory, first),
  })).sort(compareFolders)
  const together = ranked.filter(folder => folder.photos.length && folder.videos.length)
  if (together.length) {
    const best = together[0]
    for (const folder of together) {
      if (compareFolders(folder, best) !== 0) break
      const match = matchFilenames(folder)
      if (match) return match
    }
    return { photoPath: best.photos[0], videoPath: best.videos[0] }
  }

  // Split exports may have no folder with both parts. Rank each side and
  // prefer a matching filename stem among equally ranked alternatives.
  function bestFiles(part: keyof Candidates) {
    const available = ranked.filter(folder => folder[part].length)
    return available.filter(folder => compareFolders(folder, available[0]) === 0).flatMap(folder => folder[part])
  }
  const photos = bestFiles('photos')
  const videos = bestFiles('videos')
  return matchFilenames({ photos, videos }) ?? { photoPath: photos[0], videoPath: videos[0] }
}
