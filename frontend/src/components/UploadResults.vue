<script setup lang="ts">
import { computed, onUnmounted, ref } from 'vue'
import { Check, ChevronDown, Copy } from '@lucide/vue'
import { toast } from 'vue-sonner'
import { uploadManager } from '../utils/UploadManager'
import Button from './ui/button/Button.vue'

const { results } = uploadManager.state
const groups = computed(() => [
  { label: 'Uploaded', count: results.success.length },
  { label: 'Failed', count: results.fail.length },
  { label: 'Skipped', count: results.skipped.length },
  { label: 'Warnings', count: results.warnings.length },
])
const hasDetails = computed(() => results.fail.length + results.skipped.length + results.warnings.length > 0)
const noMatches = computed(() => results.warnings.find(warning => warning.code === 'no-matching-files'))
const detailsOpen = ref(false)
const visibleCount = ref(40)
const details = computed(() => {
  const failures = results.fail.slice(0, visibleCount.value).map(reason => ({ label: 'Failed', reason, paths: [] as string[] }))
  const warnings = results.warnings.slice(0, visibleCount.value - failures.length).map(item => ({ label: 'Warning', ...item }))
  const skipped = results.skipped.slice(0, visibleCount.value - failures.length - warnings.length).map(item => ({ label: 'Skipped', ...item }))
  return [...failures, ...warnings, ...skipped]
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
        <dd :class="{ 'text-destructive': group.label === 'Failed' && group.count }">
          {{ group.count }}
        </dd>
      </div>
    </dl>
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
        <ul
          v-if="detailsOpen"
          class="result-details"
        >
          <li
            v-for="(item, index) in details"
            :key="index"
          >
            <strong>{{ item.label }}</strong> {{ item.reason }}
            <small v-if="item.paths.length">{{ item.paths.join(', ') }}</small>
          </li>
          <li v-if="details.length < results.fail.length + results.skipped.length + results.warnings.length">
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
