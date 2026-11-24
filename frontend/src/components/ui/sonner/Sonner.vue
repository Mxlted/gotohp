<script lang="ts" setup>
import type { ToasterProps } from "vue-sonner"
import { Toaster as Sonner } from "vue-sonner"

const props = defineProps<ToasterProps>()
</script>

<template>
  <Sonner
    class="toaster group"
    v-bind="props"
    :style="{
      '--normal-bg': 'var(--popover)',
      '--normal-text': 'var(--popover-foreground)',
      '--normal-border': 'var(--border)',
      '--info-bg': 'var(--accent)',
      '--info-text': 'var(--primary)',
      '--success-bg': 'color-mix(in srgb, var(--success) 8%, var(--card))',
      '--success-text': 'var(--success)',
      '--warning-bg': 'var(--warning-surface)',
      '--warning-text': 'var(--warning)',
      '--error-bg': 'color-mix(in srgb, var(--destructive) 8%, var(--card))',
      '--error-text': 'var(--destructive)',
    }"
  />
</template>
