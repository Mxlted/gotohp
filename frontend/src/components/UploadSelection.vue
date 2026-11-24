<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { Clapperboard, Files, Image as ImageIcon, List, X } from '@lucide/vue'
import Button from './ui/button/Button.vue'

const props = defineProps<{ paths: string[], disabled: boolean }>()
const emit = defineEmits<{ remove: [path: string], clear: [] }>()

const visibleCount = ref(40)
const heading = ref<HTMLElement>()
const list = ref<HTMLUListElement>()
watch(() => props.paths.length > 0, (hasSelection) => {
  if (!hasSelection) visibleCount.value = 40
  heading.value?.focus()
}, { flush: 'post' })

const selections = computed(() => props.paths.slice(0, visibleCount.value).map(path => {
  const trimmed = path.replace(/[\\/]+$/, '')
  const separator = Math.max(trimmed.lastIndexOf('/'), trimmed.lastIndexOf('\\'))
  const name = separator < 0 ? path : trimmed.slice(separator + 1)
  const parent = trimmed.slice(0, separator + 1)
  let icon = Files
  if (/\.(jpe?g|png|webp|heic|heif|gif|avif|tiff?|dng|raw)$/i.test(name)) icon = ImageIcon
  if (/\.(mov|mp4|m4v|avi|mkv|webm|3gp)$/i.test(name)) icon = Clapperboard
  return { path, name, parent, icon }
}))

async function removeSelection(path: string, index: number) {
  emit('remove', path)
  await nextTick()
  const buttons = list.value?.querySelectorAll<HTMLButtonElement>('.remove-path')
  if (buttons?.length) buttons[Math.min(index, buttons.length - 1)].focus()
}
</script>

<template>
  <section
    class="panel queue-panel"
    aria-labelledby="queue-title"
  >
    <header class="panel-heading">
      <h2
        id="queue-title"
        ref="heading"
        class="panel-title"
        tabindex="-1"
      >
        Upload queue
      </h2>
      <span
        v-if="paths.length"
        class="count-label"
        role="status"
      >{{ paths.length }} selected</span>
      <Button
        v-if="paths.length"
        class="ml-auto"
        variant="ghost"
        size="sm"
        :disabled="disabled"
        @click="$emit('clear')"
      >
        Clear
      </Button>
    </header>
    <template v-if="paths.length">
      <ul
        ref="list"
        class="queue-list"
        aria-label="Selected files and folders"
        tabindex="0"
      >
        <li
          v-for="(item, index) in selections"
          :key="item.path"
          class="queue-row"
        >
          <span class="file-icon"><component
            :is="item.icon"
            aria-hidden="true"
          /></span>
          <div
            class="file-label"
            :title="item.path"
          >
            <span
              class="file-name"
              dir="auto"
            >{{ item.name }}</span>
            <span
              v-if="item.parent"
              class="file-parent"
              dir="auto"
            >{{ item.parent }}</span>
          </div>
          <Button
            size="icon"
            variant="ghost"
            class="remove-path"
            :disabled="disabled"
            :aria-label="'Remove ' + item.path"
            :title="'Remove ' + item.name"
            @click="removeSelection(item.path, index)"
          >
            <X aria-hidden="true" />
          </Button>
        </li>
        <li
          v-if="paths.length > visibleCount"
          class="px-4 py-2"
        >
          <Button
            variant="outline"
            size="sm"
            @click="visibleCount += 40"
          >
            Show more selections
          </Button>
        </li>
      </ul>
      <p class="queue-note">
        Folders are scanned when you start the upload.
      </p>
    </template>
    <div
      v-else
      class="queue-empty"
    >
      <span class="file-icon"><List aria-hidden="true" /></span>
      <div>
        <p>No files selected</p>
        <span>Folders are scanned when you start the upload.</span>
      </div>
    </div>
  </section>
</template>
