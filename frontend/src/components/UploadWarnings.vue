<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { AlertTriangle, Search } from '@lucide/vue'
import type { PreflightWarning } from '../utils/UploadManager'
import Button from './ui/button/Button.vue'

const props = defineProps<{ warnings: PreflightWarning[] }>()
const search = ref('')
const query = computed(() => search.value.trim().toLowerCase())
const matches = computed(() => {
  if (!query.value) return props.warnings
  return props.warnings.filter(warning =>
    warning.Message.toLowerCase().includes(query.value) ||
    warning.Code.toLowerCase().includes(query.value) ||
    warning.Paths?.some(path => path.toLowerCase().includes(query.value)))
})
const visibleCount = ref(40)
watch(query, () => { visibleCount.value = 40 })
</script>

<template>
  <section
    class="panel"
    aria-labelledby="warnings-heading"
  >
    <header class="panel-heading warnings-heading">
      <h2
        id="warnings-heading"
        class="panel-title flex items-center gap-2"
      >
        <AlertTriangle
          aria-hidden="true"
          class="size-4 text-warning"
        />
        Warnings
        <span class="count-label">{{ query ? matches.length + ' of ' : '' }}{{ warnings.length }}</span>
      </h2>
      <label class="warning-search">
        <Search aria-hidden="true" />
        <input
          v-model="search"
          type="search"
          class="text-field"
          aria-label="Search warnings by message, code, or file path"
          placeholder="Search warnings"
        >
      </label>
    </header>
    <ul
      v-if="matches.length"
      class="result-details"
      tabindex="0"
      aria-label="Matching warnings"
    >
      <li
        v-for="(warning, index) in matches.slice(0, visibleCount)"
        :key="index"
      >
        {{ warning.Message }}
        <small
          v-for="path in warning.Paths"
          :key="path"
        >{{ path }}</small>
      </li>
      <li v-if="visibleCount < matches.length">
        <Button
          variant="outline"
          size="sm"
          @click="visibleCount += 40"
        >
          Show more warnings
        </Button>
      </li>
    </ul>
    <p
      v-else
      class="result-note"
      role="status"
    >
      No warnings match your search.
    </p>
  </section>
</template>

<style scoped>
.warnings-heading { flex-wrap: wrap; gap: 8px 16px; background: var(--warning-surface); }
.warning-search { position: relative; width: min(100%, 13rem); margin-left: auto; }
.warning-search svg { position: absolute; top: 50%; left: 9px; width: 14px; height: 14px; transform: translateY(-50%); color: var(--muted-foreground); pointer-events: none; }
.warning-search input { height: 32px; padding-left: 30px; font-size: 13px; }
.result-details:focus-visible { outline-offset: -3px; }
</style>
