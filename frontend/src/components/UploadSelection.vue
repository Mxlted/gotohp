<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { X } from '@lucide/vue'
import Button from './ui/button/Button.vue'
import { Input } from './ui/input'
import { Label } from './ui/label'

defineProps<{ paths: string[], disabled: boolean }>()
defineEmits<{ remove: [path: string] }>()
const destination = defineModel<string>('destination', { required: true })
const album = defineModel<string>('album', { required: true })
const visibleCount = ref(40)
const heading = ref<HTMLElement>()
onMounted(() => heading.value?.focus())
</script>

<template>
  <div class="flex flex-col gap-3">
    <div>
      <h1
        ref="heading"
        tabindex="-1"
        class="text-xl font-semibold outline-none"
      >
        Review upload
      </h1>
      <p class="mt-1 text-sm text-muted-foreground">
        {{ paths.length }} {{ paths.length === 1 ? 'selection' : 'selections' }}.
        Folders are scanned when you start.
      </p>
    </div>
    <ul
      aria-label="Selected files and folders"
      class="max-h-40 overflow-y-auto divide-y rounded-lg border"
    >
      <li
        v-for="path in paths.slice(0, visibleCount)"
        :key="path"
        class="flex items-center gap-2 px-3 py-1"
      >
        <span
          class="min-w-0 flex-1 break-all text-sm"
          dir="auto"
        >{{ path }}</span>
        <Button
          size="icon"
          variant="ghost"
          class="size-8 shrink-0"
          :disabled="disabled"
          :aria-label="`Remove ${path}`"
          @click="$emit('remove', path)"
        >
          <X aria-hidden="true" />
        </Button>
      </li>
      <li
        v-if="paths.length > visibleCount"
        class="px-3 py-1"
      >
        <Button
          variant="link"
          size="sm"
          @click="visibleCount += 40"
        >
          Show more selections
        </Button>
      </li>
    </ul>
    <div class="flex flex-col gap-1.5">
      <Label for="upload-destination">Destination</Label>
      <select
        id="upload-destination"
        v-model="destination"
        :disabled="disabled"
        class="h-9 w-full rounded-md border bg-background px-2 text-sm [color-scheme:dark]"
      >
        <option value="regular">
          Photo library
        </option>
        <option value="album">
          Named album
        </option>
        <option value="auto-album">
          Albums from folder names
        </option>
      </select>
    </div>
    <div
      v-if="destination === 'album'"
      class="flex flex-col gap-1.5"
    >
      <Label for="album-input">Album name or key</Label>
      <Input
        id="album-input"
        v-model="album"
        :disabled="disabled"
      />
    </div>
  </div>
</template>
