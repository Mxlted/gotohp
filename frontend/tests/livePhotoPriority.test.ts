import assert from 'node:assert/strict'
import test from 'node:test'
import { livePhotoDirectory, livePhotoFolders, suggestLivePhotoPair } from '../src/utils/livePhotoPriority.ts'

function files(folder: string, name = 'IMG_0421') {
  return { photoPath: folder + '/' + name + '.HEIC', videoPath: folder + '/' + name + '.MOV' }
}

function group(...copies: ReturnType<typeof files>[]) {
  return { photos: copies.map(pair => pair.photoPath), videos: copies.map(pair => pair.videoPath) }
}

test('folder order chooses year or named album and falls back to unnumbered copies', () => {
  const year = files('/takeout/pHoToS FrOm 2026')
  const album = files('/takeout/Family & friends')
  const numberedAlbum = files('/takeout/Family & friends(1)')
  const numberedYear = files('/takeout/Photos From 2026 (1)')
  const candidates = group(numberedAlbum, numberedYear, album, year)
  assert.deepEqual(suggestLivePhotoPair(candidates, 'year'), year)
  assert.deepEqual(suggestLivePhotoPair(candidates, 'album'), album)
  assert.deepEqual(suggestLivePhotoPair(group(numberedYear, album), 'year'), album)
  assert.deepEqual(suggestLivePhotoPair(group(numberedAlbum, year), 'album'), year)
  const bareYear = files('/takeout/2026')
  assert.deepEqual(suggestLivePhotoPair(group(album, bareYear), 'year'), bareYear)
})

test('numbered fallback uses numeric order and leaves descriptive parentheses intact', () => {
  const early = files('/albums/Family (2)')
  const late = files('/albums/Family(10)')
  const named = files('/albums/Family (reunion)')
  assert.deepEqual(suggestLivePhotoPair(group(late, early), 'album'), early)
  assert.deepEqual(suggestLivePhotoPair(group(late, early, named), 'album'), named)
})

test('looks for a complete folder first and considers the full directory path', () => {
  const album = files('/one/Family')
  const year = files('/one/Photos from 2025')
  const otherExport = files('/two/Photos from 2025')
  const candidates = { photos: [year.photoPath, album.photoPath], videos: [otherExport.videoPath, album.videoPath] }
  assert.deepEqual(suggestLivePhotoPair(candidates, 'year'), album)
})

test('MP4 companions drive the folder indicators and take priority over folder names', () => {
  const folders = ['Photos from 2026', 'Family & friends', 'Family & friends(1)']
  const copies = folders.map(folder => ({
    photoPath: 'C:\\Export\\' + folder + '\\IMG_0421.HEIC',
    videoPath: 'c:/export/' + folder + '/IMG_0421.MP4',
  }))
  const all = group(...copies)
  const companions = livePhotoFolders(all)
  for (const copy of copies) {
    assert.deepEqual(companions.get(livePhotoDirectory(copy.photoPath))?.videos, [copy.videoPath])
  }
  assert.deepEqual(suggestLivePhotoPair(all, 'year'), copies[0])
  assert.deepEqual(suggestLivePhotoPair(all, 'album'), copies[1])

  const onlyNumbered = { photos: all.photos, videos: [copies[2].videoPath] }
  const partial = livePhotoFolders(onlyNumbered)
  assert.deepEqual(partial.get(livePhotoDirectory(copies[0].photoPath))?.videos, [])
  assert.deepEqual(partial.get(livePhotoDirectory(copies[1].photoPath))?.videos, [])
  assert.deepEqual(partial.get(livePhotoDirectory(copies[2].photoPath))?.videos, [copies[2].videoPath])
  assert.deepEqual(suggestLivePhotoPair(onlyNumbered, 'year'), copies[2])
  assert.deepEqual(suggestLivePhotoPair(onlyNumbered, 'album'), copies[2])
})

test('split exports rank each component and match filenames within tied priorities', () => {
  const year = files('/one/Photos From 2026')
  const album = files('/two/Family')
  const copy = files('/two/Family(1)')
  const candidates = {
    photos: ['/one/Photos From 2026/Edit.JPG', year.photoPath],
    videos: [copy.videoPath, album.videoPath],
  }
  assert.deepEqual(suggestLivePhotoPair(candidates, 'year'), { photoPath: year.photoPath, videoPath: album.videoPath })
  assert.equal(suggestLivePhotoPair({ photos: [], videos: candidates.videos }, 'album'), undefined)
  assert.equal(suggestLivePhotoPair({ photos: candidates.photos, videos: [] }, 'year'), undefined)
})

test('a manual choice constrains the suggested companion on either side', () => {
  const year = files('/takeout/Photos From 2026')
  const album = files('/takeout/Family & friends(1)')
  const candidates = group(year, album)
  assert.deepEqual(suggestLivePhotoPair(candidates, 'year', { photoPath: album.photoPath }), album)
  assert.deepEqual(suggestLivePhotoPair(candidates, 'year', { videoPath: album.videoPath }), album)
  assert.deepEqual(suggestLivePhotoPair(candidates, 'year', { photoPath: year.photoPath, videoPath: album.videoPath }),
    { photoPath: year.photoPath, videoPath: album.videoPath })
})

test('Windows paths retain their spelling and a matching basename resolves a tie', () => {
  const photoPath = 'C:\\Takeout\\Photos from 2026\\IMG_0421.HEIC'
  const videoPath = 'c:\\TAKEOUT\\Photos From 2026\\img_0421.mov'
  const candidates = {
    photos: ['C:\\Takeout\\Photos From 2025\\edit.jpg', 'C:\\Takeout\\Photos from 2026\\edit.jpg', photoPath],
    videos: ['C:\\Takeout\\Photos From 2025\\other.MOV', videoPath],
  }
  assert.deepEqual(suggestLivePhotoPair(candidates, 'year'), { photoPath, videoPath })
})
