<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ArrowDown, ArrowUp, ChevronDown } from '@lucide/vue'
import { useStorage } from '@vueuse/core'
import { toast } from 'vue-sonner'
import type { LivePhotoReview } from '../../bindings/app/internal/gui/models'
import { uploadManager } from '../utils/UploadManager'
import { livePhotoDirectory, livePhotoFileName, livePhotoFolders, suggestLivePhotoPair, type FolderPriority } from '../utils/livePhotoPriority'
import Button from './ui/button/Button.vue'

const props = defineProps<{ review: LivePhotoReview, disabled: boolean }>()
const index = ref(0)
const heading = ref<HTMLElement>()
const submitting = ref(false)
const firstFolder = useStorage<FolderPriority>('gotohp-live-photo-folder-priority', 'year')
const folderTypes = {
  year: { label: 'Year folders', example: 'Photos From 2026' },
  album: { label: 'Album folders', example: 'Family & friends' },
}
const priorityOrder = computed<FolderPriority[]>(() => firstFolder.value === 'album' ? ['album', 'year'] : ['year', 'album'])
const prioritiesOpen = ref(false)
const autoPickMessage = ref('')
const choices = ref(props.review.groups.map(group => ({
  contentIdentifier: group.contentIdentifier,
  photoPath: group.photos.length === 1 ? group.photos[0] : '',
  videoPath: group.videos.length === 1 ? group.videos[0] : '',
  manual: { photoPath: false, videoPath: false },
  decision: 'pending' as 'pending' | 'use' | 'skip',
})))
const choice = computed(() => choices.value[index.value])
const group = computed(() => props.review.groups[index.value])
const folders = computed(() => livePhotoFolders(group.value))
const folderSummary = computed(() => {
  const { photos, videos } = group.value
  if (!videos.length) return 'This batch contains only still images for this Live Photo.'
  if (!photos.length) return 'This batch contains only motion videos for this Live Photo.'
  let matched = 0
  for (const folder of folders.value.values()) {
    if (folder.videos.length) matched += folder.photos.length
  }
  if (!matched) return 'The matching stills and motion videos in this batch are in different folders.'
  if (photos.length === 1) return 'The still has a matching motion video in its folder.'
  if (matched === photos.length) return `All ${matched} stills have a matching motion video in their folder.`
  if (matched === 1) return `1 of ${photos.length} stills has a matching motion video in its folder.`
  return `${matched} of ${photos.length} stills have a matching motion video in their folder.`
})
const parts = computed(() => [
  { field: 'photoPath', label: 'Still image', paths: group.value.photos, missing: 'Add the matching still image in a later upload.' },
  { field: 'videoPath', label: 'Motion video', paths: group.value.videos, missing: 'Add the matching MOV or MP4 in a later upload.' },
] as const)
const lastGroup = computed(() => index.value + 1 === choices.value.length)
const busy = computed(() => submitting.value || props.disabled)
const canPair = computed(() => !!choice.value.photoPath && !!choice.value.videoPath)
const sameFolder = computed(() => livePhotoDirectory(choice.value.photoPath) === livePhotoDirectory(choice.value.videoPath))
const earlierPairs = computed(() => choices.value.slice(0, index.value).filter(item => item.decision === 'use').length)
const skipLabel = computed(() => {
  if (!lastGroup.value) return 'Skip group'
  return earlierPairs.value ? 'Skip and upload earlier pairs' : 'Finish without a pair'
})

function videoHint(photoPath: string) {
  const videos = folders.value.get(livePhotoDirectory(photoPath))?.videos
  if (!videos?.length) return 'Matching video in another folder'
  if (videos.length === 1) return `Video in this folder · ${livePhotoFileName(videos[0])}`
  return `${videos.length} matching videos in this folder`
}

function changePriority() {
  firstFolder.value = priorityOrder.value[1]
  autoPickMessage.value = ''
}

function autoPickPairs() {
  if (busy.value) return
  let count = 0
  for (const [i, draft] of choices.value.entries()) {
    if (draft.decision !== 'pending' || (draft.manual.photoPath && draft.manual.videoPath)) continue
    const pair = suggestLivePhotoPair(props.review.groups[i], priorityOrder.value[0], {
      photoPath: draft.manual.photoPath ? draft.photoPath : undefined,
      videoPath: draft.manual.videoPath ? draft.videoPath : undefined,
    })
    if (pair) {
      Object.assign(draft, pair)
      count++
    }
  }
  autoPickMessage.value = count
    ? `Preselected ${count} ${count === 1 ? 'pair' : 'pairs'} for review.`
    : 'No remaining pairs need a suggestion.'
}

async function advance(include: boolean, finish = false) {
  if (busy.value || (include && !canPair.value)) return
  choice.value.decision = include ? 'use' : 'skip'
  if (!lastGroup.value && !finish) {
    index.value++
    return
  }
  submitting.value = true
  try {
    const pairs = choices.value.slice(0, index.value + 1)
      .filter(item => item.decision === 'use')
      .map(({ contentIdentifier, photoPath, videoPath }) => ({ contentIdentifier, photoPath, videoPath }))
    await uploadManager.submitLivePhotoPairs(props.review.id, pairs)
  } catch (error) {
    toast.error('Could not submit Live Photo choices', { description: String(error) })
  } finally {
    submitting.value = false
  }
}

onMounted(() => heading.value?.focus())
watch(index, () => heading.value?.focus(), { flush: 'post' })
</script>

<template>
  <section
    class="panel"
    aria-labelledby="live-photo-picker"
  >
    <header class="panel-heading">
      <h2
        id="live-photo-picker"
        ref="heading"
        class="panel-title"
        tabindex="-1"
      >
        Choose a Live Photo pair
        <span
          v-if="choices.length > 1"
          class="count-label"
        >Group {{ index + 1 }} of {{ choices.length }}</span>
      </h2>
    </header>
    <div class="pair-fields">
      <p>{{ folderSummary }}</p>
      <div class="folder-priority">
        <div class="priority-toolbar">
          <button
            type="button"
            class="priority-toggle"
            :aria-expanded="prioritiesOpen"
            aria-controls="folder-priority-order"
            :disabled="busy"
            @click="prioritiesOpen = !prioritiesOpen"
          >
            <ChevronDown
              aria-hidden="true"
              :class="{ 'rotate-180': prioritiesOpen }"
            />
            <span>Folder priority <small>{{ folderTypes[priorityOrder[0]].label }} first</small></span>
          </button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            :disabled="busy"
            @click="autoPickPairs"
          >
            Auto-pick pairs
          </Button>
        </div>
        <div
          v-show="prioritiesOpen"
          id="folder-priority-order"
          class="priority-editor"
        >
          <ol aria-label="Folder priority, highest first">
            <li
              v-for="(kind, position) in priorityOrder"
              :key="kind"
            >
              <span class="priority-position">{{ position + 1 }}.</span>
              <span class="priority-name">{{ folderTypes[kind].label }} <small>{{ folderTypes[kind].example }}</small></span>
              <Button
                type="button"
                size="icon"
                variant="ghost"
                :disabled="busy"
                :aria-label="`Move ${folderTypes[kind].label.toLowerCase()} ${position ? 'up' : 'down'}`"
                @click="changePriority"
              >
                <component
                  :is="position ? ArrowUp : ArrowDown"
                  aria-hidden="true"
                />
              </Button>
            </li>
          </ol>
          <p>Auto-pick prefers folders containing both the still and its motion video. Numbered copies such as Family &amp; friends(1) come last among those folders.</p>
        </div>
        <p
          role="status"
          class="priority-status"
        >
          {{ autoPickMessage }}
        </p>
      </div>
      <fieldset
        v-for="part in parts"
        :key="part.field"
        :disabled="busy"
      >
        <legend class="field-label">
          {{ part.label }}
        </legend>
        <label
          v-for="path in part.paths"
          :key="path"
          class="pair-option"
        >
          <input
            v-model="choice[part.field]"
            type="radio"
            :name="part.field"
            :value="path"
            @change="choice.manual[part.field] = true"
          >
          <span>
            <strong class="file-name">{{ livePhotoFileName(path) }}</strong>
            <small class="file-parent">{{ path }}</small>
            <small
              v-if="part.field === 'photoPath' && group.videos.length"
              class="folder-hint"
            >{{ videoHint(path) }}</small>
          </span>
        </label>
        <p v-if="!part.paths.length">
          {{ part.missing }}
        </p>
      </fieldset>
      <p
        role="status"
        class="selected-location"
        :class="{ 'same-folder': sameFolder }"
      >
        {{ canPair ? (sameFolder ? 'Selected still and video are in the same folder.' : 'Selected still and video are in different folders.') : '' }}
      </p>
    </div>
    <div class="pair-actions">
      <Button
        v-if="index > 0"
        variant="ghost"
        size="sm"
        :disabled="busy"
        @click="index--"
      >
        Back
      </Button>
      <Button
        v-if="!lastGroup"
        variant="ghost"
        size="sm"
        :disabled="busy"
        @click="advance(false, true)"
      >
        {{ earlierPairs ? 'Skip remaining and upload reviewed pairs' : 'Skip remaining groups' }}
      </Button>
      <Button
        variant="outline"
        size="sm"
        :disabled="busy"
        @click="advance(false)"
      >
        {{ skipLabel }}
      </Button>
      <Button
        v-if="group.photos.length && group.videos.length"
        size="sm"
        :disabled="busy || !canPair"
        @click="advance(true)"
      >
        {{ lastGroup ? (earlierPairs ? 'Upload selected pairs' : 'Upload selected pair') : 'Use pair and continue' }}
      </Button>
    </div>
  </section>
</template>

<style scoped>
.panel-title { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; }
.pair-fields { display: grid; gap: 14px; padding: 16px 20px; }
.pair-fields p { font-size: 13px; color: var(--muted-foreground); }
.pair-fields fieldset { min-width: 0; }
.folder-priority { border: 1px solid var(--border); border-radius: 6px; }
.priority-toolbar { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 6px 12px; padding: 6px 10px; }
.priority-toggle { display: inline-flex; align-items: center; gap: 6px; padding: 4px 0; font-size: 13px; font-weight: 600; text-align: start; }
.priority-toggle svg { width: 15px; height: 15px; }
.folder-priority small { color: var(--muted-foreground); font-size: 12px; font-weight: 400; }
.priority-toggle small { display: block; }
.priority-editor { padding: 4px 12px 10px; border-top: 1px solid var(--border); }
.priority-editor li { display: flex; align-items: center; gap: 8px; font-size: 13px; }
.priority-position { font: 12px var(--mono); color: var(--muted-foreground); }
.priority-name { flex: 1; }
.priority-name small { display: block; }
.priority-editor p { margin-top: 6px; }
.priority-status:not(:empty) { padding: 6px 12px; border-top: 1px solid var(--border); }
.pair-option { display: flex; align-items: start; gap: 9px; padding: 6px 8px; border: 1px solid transparent; border-radius: 5px; cursor: pointer; }
.pair-option:hover { border-color: var(--border); }
.pair-option:has(:checked) { background: var(--tint); }
.pair-option input { flex: none; margin-top: 4px; accent-color: var(--primary); }
.pair-option span { min-width: 0; }
.folder-hint { display: block; margin-top: 3px; font-size: 12px; overflow-wrap: anywhere; }
.selected-location:empty { display: none; }
.selected-location.same-folder { color: var(--success); }
.pair-actions { display: flex; flex-wrap: wrap; justify-content: end; gap: 8px; padding: 12px 16px; border-top: 1px solid var(--border); }
</style>
