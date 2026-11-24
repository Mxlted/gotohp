<script setup lang="ts">
import { computed, onUnmounted, ref } from 'vue'
import { Check, ChevronDown, Copy, RotateCw } from '@lucide/vue'
import { toast } from 'vue-sonner'
import { uploadManager } from '../utils/UploadManager'
import { incompleteLivePhotoPaths, isSkippedUploadWarning } from '../utils/uploadResults'
import Button from './ui/button/Button.vue'
import UploadWarnings from './UploadWarnings.vue'

defineProps<{ disabled: boolean }>()
defineEmits<{ retry: [paths: string[]] }>()
const { state } = uploadManager
const { results } = state
const incompletePaths = computed(() => incompleteLivePhotoPaths(results))
const groups = computed(() => [
  { label: 'Uploaded', count: results.success.length, color: 'text-success' },
  { label: 'Already in library', count: state.alreadyInLibrary, color: 'text-foreground' },
  { label: 'Other skips', count: results.skipped.length - state.alreadyInLibrary, color: 'text-foreground' },
  { label: 'Failed', count: results.fail.length, color: 'text-destructive' },
  { label: 'Warnings', count: state.warnings.length, color: 'text-warning' },
])
// Skips reported as warnings already have a searchable row with all their paths.
const detailSkips = computed(() => results.skipped.filter(item => !isSkippedUploadWarning(item.code)))
const detailCount = computed(() => results.fail.length + detailSkips.value.length)
const hasDetails = computed(() => detailCount.value + state.warnings.length > 0)
const noMatches = computed(() => results.warnings.find(warning => warning.code === 'no-matching-files'))
const detailsOpen = ref(false)
const visibleCount = ref(40)
const details = computed(() => {
  const failures = results.fail.slice(0, visibleCount.value).map(reason => ({ label: 'Failed', reason, paths: [] as string[] }))
  const skipped = detailSkips.value.slice(0, visibleCount.value - failures.length).map(item => ({
    label: item.code === 'remote-duplicate' ? 'Already in library' : 'Skipped', ...item,
  }))
  return [...failures, ...skipped]
})

const copied = ref(false)
let copyReset: ReturnType<typeof setTimeout> | undefined
async function copyResults() {
  if (!await uploadManager.copyResultsAsJson()) {
    toast.error('Could not copy upload results')
    return
  }
  copied.value = true
  clearTimeout(copyReset)
  copyReset = setTimeout(() => { copied.value = false }, 1500)
}
onUnmounted(() => clearTimeout(copyReset))
</script>

<template>
  <section
    class="panel results-panel"
    aria-labelledby="results-title"
  >
    <header class="panel-heading">
      <h2
        id="results-title"
        tabindex="-1"
        class="panel-title"
      >
        Upload results
      </h2>
      <Button
        variant="outline"
        size="sm"
        class="ml-auto"
        @click="copyResults"
      >
        <component
          :is="copied ? Check : Copy"
          aria-hidden="true"
        />
        {{ copied ? 'Copied!' : 'Copy as JSON' }}
      </Button>
    </header>
    <dl class="result-counts">
      <div
        v-for="group in groups"
        :key="group.label"
      >
        <dt>{{ group.label }}</dt>
        <dd :class="group.count ? group.color : 'text-muted-foreground'">
          {{ group.count }}
        </dd>
      </div>
    </dl>
    <div
      v-if="incompletePaths.length"
      class="retry-incomplete"
    >
      <Button
        variant="outline"
        size="sm"
        :disabled="disabled"
        @click="$emit('retry', incompletePaths)"
      >
        <RotateCw aria-hidden="true" />
        Retry {{ incompletePaths.length }} incomplete {{ incompletePaths.length === 1 ? 'file' : 'files' }}
      </Button>
      <p>Review these files and add any missing companions before uploading.</p>
    </div>
    <p
      v-if="noMatches"
      class="result-note"
    >
      {{ noMatches.reason }}
    </p>
    <p
      v-else-if="groups.every(group => !group.count)"
      class="result-note"
    >
      No items were processed.
    </p>
    <template v-if="hasDetails">
      <button
        type="button"
        class="result-disclosure"
        :aria-expanded="detailsOpen"
        aria-controls="result-details"
        @click="detailsOpen = !detailsOpen"
      >
        <ChevronDown
          :class="{ 'rotate-180': detailsOpen }"
          aria-hidden="true"
        />
        {{ detailsOpen ? 'Hide details' : 'Show details' }}
      </button>
      <div
        id="result-details"
        :hidden="!detailsOpen"
      >
        <UploadWarnings
          v-if="state.warnings.length"
          :warnings="state.warnings"
          class="result-warnings"
        />
        <ul
          v-if="detailCount"
          class="result-details"
        >
          <li
            v-for="(item, index) in details"
            :key="index"
          >
            <strong>{{ item.label }}</strong> {{ item.reason }}
            <small v-if="item.paths.length">{{ item.paths.join(', ') }}</small>
          </li>
          <li v-if="details.length < detailCount">
            <Button
              variant="outline"
              size="sm"
              @click="visibleCount += 40"
            >
              Show more results
            </Button>
          </li>
        </ul>
      </div>
    </template>
  </section>
</template>

<style scoped>
.retry-incomplete { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 12px; padding: 0 20px 16px; }
.retry-incomplete p { font-size: 13px; color: var(--muted-foreground); }
.result-warnings { border-width: 1px 0 0; border-radius: 0; }
</style>
