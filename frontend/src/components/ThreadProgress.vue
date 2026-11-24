<script setup lang="ts">
import { computed } from 'vue'
import type { ThreadStatus } from '../utils/UploadManager'
import { formatBytes } from '../utils/formatBytes'
import { Progress } from './ui/progress'

const props = defineProps<{ thread: ThreadStatus }>()
const labels: Record<string, string> = {
  checking: 'Checking', hashing: 'Hashing', uploading: 'Uploading', finalizing: 'Finalizing',
  completed: 'Done', skipped: 'Skipped', error: 'Error',
}
const status = computed(() => {
  if (props.thread.Status === 'uploading' && props.thread.Attempt > 1) return 'Retry ' + (props.thread.Attempt - 1)
  return labels[props.thread.Status] || props.thread.Status
})
const percent = computed(() => props.thread.Status === 'uploading' && props.thread.BytesTotal > 0
  ? Math.round(props.thread.BytesUploaded / props.thread.BytesTotal * 100) : null)
</script>

<template>
  <div class="worker">
    <div class="worker-heading">
      <span
        class="worker-name"
        :title="thread.FilePath"
      >{{ thread.FileName || thread.Message }}</span>
      <span
        class="worker-status"
        :class="{
          'text-primary': thread.Status === 'uploading',
          'text-success': thread.Status === 'completed',
          'text-warning': thread.Status === 'skipped',
          'text-destructive': thread.Status === 'error',
        }"
      >{{ status }}</span>
    </div>
    <template v-if="percent !== null">
      <Progress
        :model-value="percent"
        class="my-2 h-1"
        :aria-label="'Progress for ' + thread.FileName"
      />
      <div class="worker-size">
        <span>{{ formatBytes(thread.BytesUploaded) }} / {{ formatBytes(thread.BytesTotal) }}</span>
        <span>{{ percent }}%</span>
      </div>
    </template>
    <p
      v-else-if="thread.FileName && (thread.Status === 'error' || thread.Status === 'skipped')"
      class="worker-message"
    >
      {{ thread.Message.replace(/^Error:\s*/, '') }}
    </p>
  </div>
</template>

<style scoped>
.worker { padding: 12px 20px; }
.worker + .worker { border-top: 1px solid var(--border); }
.worker-heading, .worker-size { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; }
.worker-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 14px; font-weight: 550; }
.worker-status { flex: none; font-size: 12px; }
.worker-size { font: 11px/1.5 var(--mono); color: var(--muted-foreground); }
.worker-message { margin-top: 4px; font-size: 12px; overflow-wrap: anywhere; color: var(--muted-foreground); }
</style>
