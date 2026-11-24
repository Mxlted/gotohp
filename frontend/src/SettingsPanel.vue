<script setup lang="ts">
import { nextTick, ref, onMounted, watch } from 'vue'
import { ConfigManager } from '../bindings/app/core'
import { Dialogs } from '@wailsio/runtime'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
    Tooltip,
    TooltipContent,
    TooltipTrigger,
} from '@/components/ui/tooltip'
import { Info } from '@lucide/vue'

import {
    NumberField,
    NumberFieldContent,
    NumberFieldDecrement,
    NumberFieldIncrement,
    NumberFieldInput,
} from '@/components/ui/number-field'

interface Settings {
    proxy: string
    useQuota: boolean
    saver: boolean
    recursive: boolean
    forceUpload: boolean
    pairLivePhotos: boolean
    skipIncompleteLivePhotos: boolean
    updateExistingPhotosToLive: boolean
    afterUpload: 'keep' | 'move' | 'delete'
    moveDestination: string
    disableUnsupportedFilesFilter: boolean
    setDateFromFilename: boolean
    uploadThreads: number
}

type BooleanSetting = 'forceUpload' | 'pairLivePhotos' | 'skipIncompleteLivePhotos' | 'updateExistingPhotosToLive'

const settings = ref<Settings>({
    proxy: '',
    useQuota: false,
    saver: false,
    recursive: false,
    forceUpload: false,
    pairLivePhotos: false,
    skipIncompleteLivePhotos: true,
    updateExistingPhotosToLive: false,
    afterUpload: 'keep',
    moveDestination: '',
    disableUnsupportedFilesFilter: false,
    setDateFromFilename: false,
    uploadThreads: 0
})
const isHydrating = ref(true)
const isChoosingFolder = ref(false)
const isSavingAction = ref(false)

const toggleSetting = (setting: BooleanSetting, enabled = true) => {
    if (!enabled) return
    settings.value[setting] = !settings.value[setting]
}

onMounted(async () => {
    try {
        const config = await ConfigManager.GetSettings()
        settings.value = {
            proxy: config.proxy || '',
            useQuota: config.useQuota || false,
            saver: config.saver || false,
            recursive: config.recursive || false,
            forceUpload: config.forceUpload || false,
            pairLivePhotos: config.pairLivePhotos || false,
            skipIncompleteLivePhotos: config.skipIncompleteLivePhotos ?? true,
            updateExistingPhotosToLive: config.updateExistingPhotosToLive || false,
            afterUpload: config.moveAfterUpload ? 'move' : config.deleteFromHost ? 'delete' : 'keep',
            moveDestination: config.moveDestination || '',
            disableUnsupportedFilesFilter: config.disableUnsupportedFilesFilter || false,
            setDateFromFilename: config.setDateFromFilename || false,
            uploadThreads: config.uploadThreads || 1
        }
    } finally {
        await nextTick()
        isHydrating.value = false
    }
})

// Watch for changes to proxy value and update backend
watch(() => settings.value.proxy, async (newValue) => {
    if (isHydrating.value) return
    await ConfigManager.SetProxy(newValue)
})

// Create individual watchers for each boolean setting
watch(() => settings.value.useQuota, async (newValue) => {
    if (isHydrating.value) return
    await ConfigManager.SetUseQuota(newValue)
})

watch(() => settings.value.saver, async (newValue) => {
    if (isHydrating.value) return
    await ConfigManager.SetSaver(newValue)
})

watch(() => settings.value.recursive, async (newValue) => {
    if (isHydrating.value) return
    await ConfigManager.SetRecursive(newValue)
})

watch(() => settings.value.forceUpload, async (newValue) => {
    if (isHydrating.value) return
    await ConfigManager.SetForceUpload(newValue)
})

watch(() => settings.value.pairLivePhotos, async (newValue) => {
    if (isHydrating.value) return
    await ConfigManager.SetPairLivePhotos(newValue)
    if (newValue && !settings.value.skipIncompleteLivePhotos) {
        settings.value.skipIncompleteLivePhotos = true
        await ConfigManager.SetSkipIncompleteLivePhotos(true)
    }
})

watch(() => settings.value.skipIncompleteLivePhotos, async (newValue) => {
    if (isHydrating.value) return
    await ConfigManager.SetSkipIncompleteLivePhotos(newValue)
})

watch(() => settings.value.updateExistingPhotosToLive, async (newValue) => {
    if (isHydrating.value) return
    await ConfigManager.SetUpdateExistingPhotosToLive(newValue)
})

async function saveAfterUpload(action: Settings['afterUpload'], folder: string) {
    isSavingAction.value = true
    try {
        await ConfigManager.SetAfterUploadAction(action, folder)
        settings.value.afterUpload = action
        settings.value.moveDestination = folder
    } catch (error) {
        toast.error('Could not save the after-upload setting', { description: String(error) })
    } finally {
        isSavingAction.value = false
    }
}

async function changeAfterUpload(event: Event) {
    const select = event.target as HTMLSelectElement
    await saveAfterUpload(select.value as Settings['afterUpload'], settings.value.moveDestination)
    select.value = settings.value.afterUpload
}

async function chooseMoveDestination() {
    isChoosingFolder.value = true
    try {
        const folder = await Dialogs.OpenFile({
            Title: 'Choose where to create dated folders',
            CanChooseFiles: false,
            CanChooseDirectories: true,
            CanCreateDirectories: true,
            Directory: settings.value.moveDestination || undefined,
        })
        if (folder) await saveAfterUpload('move', folder)
    } catch (error) {
        toast.error('Could not set the destination folder', { description: String(error) })
    } finally {
        isChoosingFolder.value = false
    }
}

watch(() => settings.value.disableUnsupportedFilesFilter, async (newValue) => {
    if (isHydrating.value) return
    await ConfigManager.SetDisableUnsupportedFilesFilter(newValue)
})

watch(() => settings.value.setDateFromFilename, async (newValue) => {
    if (isHydrating.value) return
    await ConfigManager.SetSetDateFromFilename(newValue)
})

watch(() => settings.value.uploadThreads, async (newValue) => {
    if (isHydrating.value) return
    if (newValue < 1) {
        settings.value.uploadThreads = 1
    } else {
        await ConfigManager.SetUploadThreads(newValue)
    }
})
</script>

<template>
  <div class="settings-list">
    <h3>Transfer</h3>
    <NumberField
      v-model="settings.uploadThreads"
      class="flex items-center justify-between"
    >
      <Label
        for="upload-threads"
        class="size-full"
      >Upload threads</Label>
      <NumberFieldContent>
        <NumberFieldDecrement
          class="cursor-pointer"
          :disabled="settings.uploadThreads <= 1"
        />
        <NumberFieldInput id="upload-threads" />
        <NumberFieldIncrement class="cursor-pointer" />
      </NumberFieldContent>
    </NumberField>
    <div class="flex items-center justify-between">
      <Label
        for="use-quota"
        class="size-full cursor-pointer"
      >Use Quota</Label>
      <Switch
        id="use-quota"
        v-model="settings.useQuota"
      />
    </div>
    <div class="flex items-center justify-between">
      <Label
        for="saver-mode"
        class="size-full cursor-pointer"
      >Storage Saver Quality</Label>
      <Switch
        id="saver-mode"
        v-model="settings.saver"
      />
    </div>
    <div class="proxy-setting">
      <Label for="proxy-url">Proxy URL</Label>
      <Input
        id="proxy-url"
        v-model="settings.proxy"
        type="text"
        placeholder="Optional"
      />
    </div>
    <h3>Files and folders</h3>
    <div class="flex items-center justify-between">
      <Label
        for="recursive"
        class="size-full cursor-pointer"
      >Recursive Directory Upload</Label>
      <Switch
        id="recursive"
        v-model="settings.recursive"
      />
    </div>
    <div class="flex items-center justify-between">
      <div
        class="flex min-w-0 flex-1 cursor-pointer items-center gap-1.5"
        @click.self="toggleSetting('forceUpload')"
      >
        <Label
          for="force-upload"
          class="cursor-pointer"
        >Force Upload</Label>
        <Tooltip>
          <TooltipTrigger as-child>
            <button
              type="button"
              class="inline-flex size-5 shrink-0 cursor-help items-center justify-center rounded-full text-muted-foreground hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
              aria-label="About Force Upload"
            >
              <Info class="size-3.5" />
            </button>
          </TooltipTrigger>
          <TooltipContent class="max-w-72">
            Bypass the Google Photos duplicate check for individual files. With Force Upload off, exact matches are skipped. Live Photo pairs always check both components.
          </TooltipContent>
        </Tooltip>
      </div>
      <Switch
        id="force-upload"
        v-model="settings.forceUpload"
      />
    </div>
    <div class="flex items-center justify-between">
      <div
        class="flex min-w-0 flex-1 cursor-pointer items-center gap-1.5 pr-4"
        @click.self="toggleSetting('pairLivePhotos')"
      >
        <Label
          for="pair-live-photos"
          class="cursor-pointer"
        >Pair Apple Live Photos</Label>
        <Tooltip>
          <TooltipTrigger as-child>
            <button
              type="button"
              class="inline-flex size-5 shrink-0 cursor-help items-center justify-center rounded-full text-muted-foreground hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
              aria-label="About Pair Apple Live Photos"
            >
              <Info class="size-3.5" />
            </button>
          </TooltipTrigger>
          <TooltipContent class="max-w-72">
            Match a still image to its MOV or MP4 companion using embedded Apple metadata and upload them as one Live Photo.
          </TooltipContent>
        </Tooltip>
      </div>
      <Switch
        id="pair-live-photos"
        v-model="settings.pairLivePhotos"
      />
    </div>
    <div
      class="flex items-center justify-between pl-4 transition-opacity"
      :class="settings.pairLivePhotos ? 'opacity-100' : 'opacity-45'"
    >
      <div
        class="flex min-w-0 flex-1 items-center gap-1.5 pr-4"
        :class="settings.pairLivePhotos ? 'cursor-pointer' : 'cursor-not-allowed'"
        @click.self="toggleSetting('skipIncompleteLivePhotos', settings.pairLivePhotos)"
      >
        <Label
          for="skip-incomplete-live-photos"
          :class="settings.pairLivePhotos ? 'cursor-pointer' : 'cursor-not-allowed'"
        >Skip Incomplete Live Photos</Label>
        <Tooltip>
          <TooltipTrigger as-child>
            <button
              type="button"
              class="inline-flex size-5 shrink-0 cursor-help items-center justify-center rounded-full text-muted-foreground hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
              aria-label="About Skip Incomplete Live Photos"
            >
              <Info class="size-3.5" />
            </button>
          </TooltipTrigger>
          <TooltipContent class="max-w-72">
            Only upload Live Photos when both photo and video are available. If either is missing,
            don't upload the remaining file separately.
          </TooltipContent>
        </Tooltip>
      </div>
      <Switch
        id="skip-incomplete-live-photos"
        v-model="settings.skipIncompleteLivePhotos"
        :disabled="!settings.pairLivePhotos"
      />
    </div>
    <div
      class="flex items-center justify-between pl-4 transition-opacity"
      :class="settings.pairLivePhotos ? 'opacity-100' : 'opacity-45'"
    >
      <div
        class="flex min-w-0 flex-1 items-center gap-1.5 pr-4"
        :class="settings.pairLivePhotos ? 'cursor-pointer' : 'cursor-not-allowed'"
        @click.self="toggleSetting('updateExistingPhotosToLive', settings.pairLivePhotos)"
      >
        <Label
          for="update-existing-photos-to-live"
          :class="settings.pairLivePhotos ? 'cursor-pointer' : 'cursor-not-allowed'"
        >Update Existing Photos to Live Photos</Label>
        <Tooltip>
          <TooltipTrigger as-child>
            <button
              type="button"
              class="inline-flex size-5 shrink-0 cursor-help items-center justify-center rounded-full text-muted-foreground hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
              aria-label="About Update Existing Photos to Live Photos"
            >
              <Info class="size-3.5" />
            </button>
          </TooltipTrigger>
          <TooltipContent class="max-w-72">
            If a static photo was already uploaded, re-upload it alongside its motion video to upgrade it to a Live Photo. Both local files are required.
          </TooltipContent>
        </Tooltip>
      </div>
      <Switch
        id="update-existing-photos-to-live"
        v-model="settings.updateExistingPhotosToLive"
        :disabled="!settings.pairLivePhotos"
      />
    </div>
    <div class="flex items-center justify-between">
      <Label
        for="filter-unsupported"
        class="size-full cursor-pointer"
      >Disable Unsupported Files Filter</Label>
      <Switch
        id="filter-unsupported"
        v-model="settings.disableUnsupportedFilesFilter"
      />
    </div>
    <div class="flex items-center justify-between">
      <Label
        for="set-date-from-filename"
        class="size-full cursor-pointer"
      >Set Upload Date from Filename</Label>
      <Switch
        id="set-date-from-filename"
        v-model="settings.setDateFromFilename"
      />
    </div>
    <h3>After upload</h3>
    <div class="flex items-center justify-between">
      <Label for="after-upload">Verified local files</Label>
      <select
        id="after-upload"
        :value="settings.afterUpload"
        class="h-8 rounded-md border border-input bg-background px-2 text-sm focus-visible:ring-2 focus-visible:ring-ring"
        :disabled="isHydrating || isSavingAction || isChoosingFolder"
        @change="changeAfterUpload"
      >
        <option value="keep">
          Keep in place
        </option>
        <option value="move">
          Move to dated folder
        </option>
        <option value="delete">
          Delete from disk
        </option>
      </select>
    </div>
    <div
      v-if="settings.afterUpload === 'move'"
      class="space-y-2"
    >
      <p class="text-sm font-medium">
        Create dated folders in
      </p>
      <p class="break-all text-sm">
        {{ settings.moveDestination || 'The parent of each selected folder' }}
      </p>
      <div class="flex flex-wrap gap-2">
        <Button
          variant="outline"
          size="sm"
          :disabled="isChoosingFolder || isSavingAction"
          @click="chooseMoveDestination"
        >
          Choose folder
        </Button>
        <Button
          v-if="settings.moveDestination"
          variant="ghost"
          size="sm"
          :disabled="isChoosingFolder || isSavingAction"
          @click="saveAfterUpload('move', '')"
        >
          Use default
        </Button>
      </div>
      <p class="text-xs text-muted-foreground">
        Keeps the subfolders inside each selection. Files already matched in Google Photos also move.
      </p>
    </div>
  </div>
</template>

<style scoped>
.settings-list { display: grid; gap: 0; padding: 4px 20px 22px; }
.settings-list h3 { margin-top: 17px; margin-bottom: 4px; color: var(--muted-foreground); font-size: 12px; font-weight: 650; }
.settings-list > div { min-height: 39px; padding-block: 5px; border-bottom: 1px solid var(--border); }
.settings-list > div:last-child { border: 0; }
.settings-list .proxy-setting { display: grid; grid-template-columns: 1fr minmax(0, 55%); gap: 10px; align-items: center; }
</style>
