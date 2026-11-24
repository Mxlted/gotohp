<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { toast } from 'vue-sonner'
import { Album, Clock, Gauge, Square, HardDrive } from '@lucide/vue'
import { Button } from './components/ui/button'
import { Progress } from './components/ui/progress'
import ThreadProgress from './components/ThreadProgress.vue'
import UploadWarnings from './components/UploadWarnings.vue'
import { uploadManager } from './utils/UploadManager'
import { formatBytes } from './utils/formatBytes'

const { state } = uploadManager
const isCancelling = ref(false)
const now = ref(Date.now())
let timer: ReturnType<typeof setInterval>
onMounted(() => { timer = setInterval(() => { now.value = Date.now() }, 1000) })
onUnmounted(() => clearInterval(timer))

const threads = computed(() => [...state.threads.values()]
  .filter(thread => thread.Status !== 'idle')
  .sort((a, b) => a.WorkerID - b.WorkerID))
const percent = computed(() => state.totalFiles ? Math.round(state.uploadedFiles / state.totalFiles * 100) : 0)
const elapsed = computed(() => {
  const seconds = Math.max(0, Math.floor((now.value - state.startTime) / 1000))
  const minutes = Math.floor(seconds / 60)
  if (minutes >= 60) return Math.floor(minutes / 60) + 'h ' + minutes % 60 + 'm'
  if (minutes) return minutes + 'm ' + seconds % 60 + 's'
  return seconds + 's'
})
const albumPercent = computed(() => state.albumStatus?.TotalItems
  ? Math.round(state.albumStatus.ItemsAdded / state.albumStatus.TotalItems * 100) : 0)

async function cancelUpload() {
  if (isCancelling.value) return
  isCancelling.value = true
  try {
    const declined = await uploadManager.cancelUpload()
    if (declined) {
      toast.error('Cancellation request was declined')
      isCancelling.value = false
    }
  } catch (error) {
    toast.error('Could not cancel upload', { description: String(error) })
    isCancelling.value = false
  }
}
</script>

<template>
  <div class="workspace-grid">
    <div class="source-column">
      <section
        class="batch-progress"
        aria-labelledby="batch-title"
      >
        <h2
          id="batch-title"
          class="panel-title"
        >
          Batch progress
        </h2>
        <template v-if="state.totalFiles">
          <p class="batch-percent">
            {{ percent }}<span>%</span>
          </p>
          <Progress
            :model-value="percent"
            aria-label="Batch progress"
          />
          <p class="batch-description">
            {{ state.uploadedFiles }} of {{ state.totalFiles }} items processed
            <span v-if="state.alreadyInLibrary"> · {{ state.alreadyInLibrary }} already in Google Photos</span>
          </p>
        </template>
        <p
          v-else
          class="scan-status"
          role="status"
        >
          Scanning your selection…
        </p>
      </section>
      <section
        class="panel"
        aria-labelledby="activity-heading"
      >
        <header class="panel-heading">
          <h2
            id="activity-heading"
            class="panel-title"
          >
            File activity
          </h2>
        </header>
        <div
          v-if="threads.length"
          class="worker-list"
          tabindex="0"
          aria-label="Upload threads"
        >
          <ThreadProgress
            v-for="thread in threads"
            :key="thread.WorkerID"
            :thread="thread"
          />
        </div>
        <p
          v-else
          class="quiet-note"
        >
          Waiting for file activity…
        </p>
      </section>
      <section
        v-if="state.albumStatus && (state.isCreatingAlbum || state.albumStatus.IsComplete)"
        class="panel"
        aria-labelledby="album-heading"
      >
        <header class="panel-heading">
          <Album
            aria-hidden="true"
            class="size-4"
          />
          <h2
            id="album-heading"
            class="panel-title"
          >
            {{ state.isCreatingAlbum ? 'Adding to album' : 'Added to album' }}
          </h2>
        </header>
        <div class="album-progress">
          <p>{{ state.albumStatus.AlbumName }}</p>
          <Progress
            :model-value="albumPercent"
            aria-label="Album progress"
          />
          <span>{{ state.albumStatus.ItemsAdded }} of {{ state.albumStatus.TotalItems }} items</span>
        </div>
      </section>
      <UploadWarnings
        v-if="state.warnings.length"
        :warnings="state.warnings"
      />
    </div>
    <aside
      class="panel"
      aria-labelledby="transfer-heading"
    >
      <header class="panel-heading">
        <h2
          id="transfer-heading"
          class="panel-title"
        >
          Transfer details
        </h2>
      </header>
      <dl class="transfer-stats">
        <div><dt><Gauge aria-hidden="true" /> Speed</dt><dd>{{ state.uploadSpeed > 0 ? formatBytes(state.uploadSpeed) + '/s' : '—' }}</dd></div>
        <div><dt><Clock aria-hidden="true" /> Elapsed</dt><dd>{{ elapsed }}</dd></div>
        <div>
          <dt><HardDrive aria-hidden="true" /> Transferred</dt><dd>
            {{ formatBytes(state.uploadedBytes) }}<template v-if="state.totalBytes">
              / {{ formatBytes(state.totalBytes) }}
            </template>
          </dd>
        </div>
      </dl>
      <div class="destination-action">
        <Button
          variant="outline"
          class="w-full"
          :disabled="isCancelling"
          @click="cancelUpload"
        >
          <Square aria-hidden="true" /> {{ isCancelling ? 'Cancelling…' : 'Cancel upload' }}
        </Button>
      </div>
    </aside>
  </div>
</template>

<style scoped>
.batch-progress { border: 1px solid var(--upload-border); border-radius: 10px; padding: 24px; background: var(--upload-surface); color: var(--upload-foreground); }
.batch-percent { margin-block: 6px 14px; font: 56px/1.1 var(--display); }
.batch-percent span { font-size: .65em; }
.batch-description, .scan-status { margin-top: 13px; color: var(--muted-foreground); font-size: 13px; }
.scan-status { margin-block: 25px; font-size: 17px; }
.quiet-note { padding: 20px; color: var(--muted-foreground); font-size: 13px; }
.worker-list { max-height: 42vh; overflow-y: auto; }
.worker-list:focus-visible { outline-offset: -3px; }
.transfer-stats { padding: 4px 20px; }
.transfer-stats > div { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 4px 12px; padding-block: 12px; }
.transfer-stats > div + div { border-top: 1px solid var(--border); }
.transfer-stats dt { display: inline-flex; gap: 7px; align-items: center; font-size: 13px; color: var(--muted-foreground); }
.transfer-stats svg { width: 15px; height: 15px; }
.transfer-stats dd, .album-progress span { font: 12px/1.5 var(--mono); }
.album-progress { display: grid; gap: 9px; padding: 14px 20px; font-size: 14px; overflow-wrap: anywhere; }
</style>
