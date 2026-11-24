<script setup lang="ts">
import { TooltipProvider } from '@/components/ui/tooltip'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
import { useColorMode } from '@vueuse/core'
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { Album, ArrowRight, Files, FolderOpen, Folders, Images, Moon, Plus, SlidersHorizontal, Sun, X } from '@lucide/vue'
import { ConfigManager } from '../bindings/app/core'
import type { UploadDestination } from '../bindings/app/core/models'
import { Clipboard, Dialogs, Events } from '@wailsio/runtime'
import Button from "./components/ui/button/Button.vue"
import GoogleAccountSelect from './components/GoogleAccountSelect.vue'
import GoogleAuthSetup from "./components/GoogleAuthSetup.vue"
import './index.css'
import SettingsPanel from "./SettingsPanel.vue"
import Upload from './Upload.vue'
import UploadSelection from './components/UploadSelection.vue'
import UploadResults from './components/UploadResults.vue'
import brandMark from './assets/gotohp.svg'
import tetons from './assets/tetons.webp'
import { uploadManager } from './utils/UploadManager'
import { incompleteLivePhotoPaths } from './utils/uploadResults'
import Toaster from './components/ui/sonner/Sonner.vue'

import { toast } from "vue-sonner"

const colorMode = useColorMode({ storageKey: 'gotohp-appearance' })
const settingsOpen = ref(false)
const workspaceHeading = ref<HTMLElement>()

const { state: uploadState } = uploadManager
const hasResults = computed(() => !uploadState.isUploading && uploadState.startTime > 0)
let showedUploadError = false

watch(() => uploadState.isUploading, running => {
  if (running) {
    showedUploadError = false
    toast.dismiss('upload-error')
  }
}, { flush: 'sync' })

const isDraggingFiles = ref(false)

const pendingFiles = ref<string[]>([])
const allowIncompletePaths = ref(new Set<string>())
let lastDestination: UploadDestination | undefined
const destination = ref('regular')
const destinations = [
  { value: 'regular', label: 'Photo library', icon: Images, dropHint: 'Add to your library.' },
  { value: 'album', label: 'Named album', icon: Album, dropHint: 'Choose an album name or key next.' },
  { value: 'auto-album', label: 'Albums from folders', icon: Folders, dropHint: 'Use folder names for the albums.' },
] as const
const isStarting = ref(false)
const isChoosing = ref(false)
let stopFileDrops: (() => void) | undefined

const selectedOption = ref('')
const options = ref<string[]>([])
const accountNeedsTokenBinding = ref<Record<string, boolean>>({})
const albumNameOrKey = ref('')
const tokenBindingEmail = ref('')
const isExtractingTokenBinding = ref(false)
const isAccountSetupOpen = ref(false)
const removingAccount = ref('')
const hasAccount = computed(() => options.value.includes(selectedOption.value))
const canUpload = computed(() => pendingFiles.value.length > 0 && hasAccount.value &&
  !uploadState.isUploading && !isStarting.value && !isChoosing.value && (destination.value !== 'album' || !!albumNameOrKey.value.trim()))
const startHint = computed(() => {
  if (!pendingFiles.value.length) return 'Add files or folders to start.'
  if (!hasAccount.value) return 'Select a Google account to start.'
  if (destination.value === 'album' && !albumNameOrKey.value.trim()) return 'Enter an album name or key to start.'
  return ''
})
const canReceiveDrop = computed(() => !uploadState.isUploading && !isStarting.value && !isChoosing.value &&
  !settingsOpen.value && !isAccountSetupOpen.value)

watch(() => uploadState.isUploading, running => {
  const heading = !running && document.getElementById('results-title')
  if (heading) heading.focus()
  else workspaceHeading.value?.focus()
}, { flush: 'post' })

watch(selectedOption, async (newValue) => {
  if (newValue) {
    try {
      await ConfigManager.SetSelected(newValue)
      await updateTokenBindingPrompt(newValue)
      console.log('Successfully updated selected value:', newValue)
    } catch (error) {
      console.error('Failed to update selected value:', error)
      toast.error('Failed to update selected account.')
    }
  } else {
    tokenBindingEmail.value = ''
  }
})

async function updateTokenBindingPrompt(email: string) {
  tokenBindingEmail.value = accountNeedsTokenBinding.value[email] ? email : ''
}

async function refreshCredentials() {
  try {
    const state = await ConfigManager.GetAccounts()
    const nextNeedsTokenBinding: Record<string, boolean> = {}
    const nextOptions = state.accounts.map(account => {
      nextNeedsTokenBinding[account.email] = account.needsTokenBinding
      return account.email
    })

    accountNeedsTokenBinding.value = nextNeedsTokenBinding
    options.value = nextOptions
    selectedOption.value = state.selected || ''
    if (state.selected) {
      await updateTokenBindingPrompt(state.selected)
    } else {
      tokenBindingEmail.value = ''
    }
  } catch (error) {
    console.error('Failed to refresh Google accounts:', error)
    toast.error('Could not refresh Google accounts', {
      description: error instanceof Error ? error.message : String(error),
    })
  }
}

async function addTokenBindingAliasFromADB() {
  if (!tokenBindingEmail.value) return

  isExtractingTokenBinding.value = true
  try {
    await ConfigManager.AddTokenBindingAliasFromADB(tokenBindingEmail.value)
    await refreshCredentials()
    tokenBindingEmail.value = ''
    toast.success('Token binding key added.')
  } catch (error) {
    console.error('Failed to add token binding key:', error)
    toast.error('Failed to add token binding key', {
      description: error instanceof Error ? error.message : String(error),
    })
  } finally {
    isExtractingTokenBinding.value = false
  }
}

async function removeCredentials(email: string) {
  removingAccount.value = email
  try {
    await ConfigManager.RemoveCredentials(email)

    const removedSelectedAccount = selectedOption.value === email
    await refreshCredentials()
    if (removedSelectedAccount && options.value.length > 0 && !selectedOption.value) {
      selectedOption.value = options.value[0]
    }
    if (!options.value.length) {
      await nextTick()
      focusAccountControl()
    }
    toast.success('Credentials removed.')
    return true
  } catch (error) {
    console.error('Failed to remove credentials:', error)
    toast.error('Failed to remove credentials.')
    return false
  } finally {
    removingAccount.value = ''
  }
}

function openAccountSetup() {
  isAccountSetupOpen.value = true
}

function focusAccountControl() {
  document.getElementById('google-account-control')?.focus()
}

onMounted(refreshCredentials)
// Global drag event handlers to detect file dragging
let dragLeaveTimeout: ReturnType<typeof setTimeout> | null = null
let dropReset: ReturnType<typeof setTimeout> | undefined

const onDragEnter = (e: DragEvent) => {
  if (!canReceiveDrop.value || !e.dataTransfer?.types.includes('Files')) return
  clearTimeout(dropReset)
  
  // Clear any pending drag leave timeout
  if (dragLeaveTimeout) {
    clearTimeout(dragLeaveTimeout)
    dragLeaveTimeout = null
  }
  
  isDraggingFiles.value = true
}

const onDragOver = (e: DragEvent) => {
  if (!e.dataTransfer?.types.includes('Files')) return
  e.preventDefault()
  
  // Clear any pending drag leave timeout - we're still dragging
  if (dragLeaveTimeout) {
    clearTimeout(dragLeaveTimeout)
    dragLeaveTimeout = null
  }
}

const onDragLeave = (e: DragEvent) => {
  if (!e.dataTransfer?.types.includes('Files')) return
  
  // Use timeout to detect if we've truly left the window
  // dragover will cancel this if we're still in the window
  if (dragLeaveTimeout) {
    clearTimeout(dragLeaveTimeout)
  }
  dragLeaveTimeout = setTimeout(() => {
    isDraggingFiles.value = false
    dragLeaveTimeout = null
  }, 50)
}

const onDrop = () => {
  // Clear any pending timeout
  if (dragLeaveTimeout) {
    clearTimeout(dragLeaveTimeout)
    dragLeaveTimeout = null
  }
  
  // Delay resetting isDraggingFiles to allow Wails to process the drop target
  // before Vue re-renders and hides the drop zones
  clearTimeout(dropReset)
  dropReset = setTimeout(() => {
    isDraggingFiles.value = false
  }, 100)
}

function stageFiles(files: string[], dropZone?: string) {
  if (!files.length || isStarting.value || uploadState.isUploading) return
  if (!pendingFiles.value.length && dropZone && destinations.some(option => option.value === dropZone)) {
    destination.value = dropZone
  }
  pendingFiles.value = [...new Set([...pendingFiles.value, ...files])]
}

async function choosePaths(folders: boolean) {
  if (isChoosing.value || isStarting.value) return
  isChoosing.value = true
  try {
    const paths = await Dialogs.OpenFile({
      Title: folders ? 'Choose folders' : 'Choose files',
      CanChooseFiles: !folders,
      CanChooseDirectories: folders,
      AllowsMultipleSelection: true,
    })
    if (paths?.length) stageFiles(paths)
  } catch (error) {
    toast.error('Could not open the file picker', { description: String(error) })
  } finally {
    isChoosing.value = false
  }
}

function clearSelection() {
  pendingFiles.value = []
  allowIncompletePaths.value.clear()
}

function removePath(path: string) {
  pendingFiles.value = pendingFiles.value.filter(item => item !== path)
  allowIncompletePaths.value.delete(path)
}

function stageIncompleteFiles(paths: string[], previous?: UploadDestination) {
  if (!paths.length || pendingFiles.value.length || isStarting.value || uploadState.isUploading) return
  stageFiles(paths)
  allowIncompletePaths.value = new Set(paths)
  if (previous) {
    selectedOption.value = options.value.includes(previous.account) ? previous.account : ''
    destination.value = previous.autoAlbum ? 'auto-album' : previous.albumName ? 'album' : 'regular'
    // A name creates a fresh album. Prefer the album key returned by the original transfer.
    albumNameOrKey.value = previous.albumName ? uploadState.albumStatus?.AlbumKeys?.at(-1) || previous.albumName : ''
  }
}

async function pasteSkippedFiles() {
  if (pendingFiles.value.length || isStarting.value || isChoosing.value || uploadState.isUploading) return
  isChoosing.value = true
  try {
    const paths = incompleteLivePhotoPaths(JSON.parse(await Clipboard.Text()))
    if (paths.length) stageIncompleteFiles(paths)
    else toast.info('This report has no skipped incomplete Live Photos.')
  } catch (error) {
    toast.error('Could not read an upload report', {
      description: error instanceof SyntaxError ? 'Use Copy as JSON in the upload results, then try again.' : String(error),
    })
  } finally {
    isChoosing.value = false
  }
}

async function confirmUpload() {
  if (!canUpload.value) return
  isStarting.value = true
  const reviewedDestination = {
    account: selectedOption.value,
    albumName: destination.value === 'album' ? albumNameOrKey.value.trim() : '',
    autoAlbum: destination.value === 'auto-album',
  }
  try {
    const cancelled = await Events.Emit('startUpload', {
      files: [...pendingFiles.value],
      destination: reviewedDestination,
      allowIncompletePaths: allowIncompletePaths.value.size ? [...allowIncompletePaths.value] : undefined,
    })
    if (cancelled) {
      toast.error('Upload request was cancelled')
      return
    }
    lastDestination = reviewedDestination
    clearSelection()
    destination.value = 'regular'
    albumNameOrKey.value = ''
  } catch (error) {
    toast.error('Could not start upload', { description: String(error) })
  } finally {
    isStarting.value = false
  }
}

// Handle album error event
const albumErrorHandler = (e: Event) => {
  const event = e as CustomEvent<{ AlbumName: string; Error: string }>
  const { AlbumName, Error } = event.detail
  // Check if it's a 404 error (album key not found)
  if (Error.includes('404')) {
    toast.error('Album not found', {
      description: `The album key "${AlbumName}" does not exist or is invalid.`,
    })
  } else {
    toast.error('Failed to create album', {
      description: `Album "${AlbumName}": ${Error}`,
    })
  }
}

const uploadErrorHandler = (e: Event) => {
  if (showedUploadError) return
  showedUploadError = true
  const event = e as CustomEvent<{ FileName: string; Message: string }>
  const { FileName, Message } = event.detail
  const errorMessage = Message.replace(/^Error:\s*/, '')
  toast.error('Upload failed', {
    id: 'upload-error',
    description: FileName ? `${FileName}: ${errorMessage}` : errorMessage,
    descriptionClass: 'max-h-16 overflow-y-auto break-all',
    duration: 10000,
  })
}

onMounted(() => {
  document.addEventListener('dragenter', onDragEnter)
  document.addEventListener('dragleave', onDragLeave)
  document.addEventListener('dragover', onDragOver)
  document.addEventListener('drop', onDrop)
  window.addEventListener('albumError', albumErrorHandler)
  window.addEventListener('uploadError', uploadErrorHandler)

  // Listen for files-dropped event from backend
  stopFileDrops = Events.On('files-dropped', ({ data }) => {
    if (canReceiveDrop.value) stageFiles(data.files, data.dropZone)
    isDraggingFiles.value = false
  })
})

onUnmounted(() => {
  stopFileDrops?.()
  clearTimeout(dropReset)
  document.removeEventListener('dragenter', onDragEnter)
  document.removeEventListener('dragleave', onDragLeave)
  document.removeEventListener('dragover', onDragOver)
  document.removeEventListener('drop', onDrop)
  window.removeEventListener('albumError', albumErrorHandler)
  window.removeEventListener('uploadError', uploadErrorHandler)
  if (dragLeaveTimeout) {
    clearTimeout(dragLeaveTimeout)
  }
})
</script>

<template>
  <div
    class="app-shell"
    :data-file-drop-target="canReceiveDrop ? '' : undefined"
  >
    <header class="app-bar">
      <div class="app-bar-inner">
        <div class="brand">
          <span class="brand-mark"><img
            :src="brandMark"
            alt=""
          ></span>
          <span class="brand-name">gotohp</span>
          <span class="brand-description">Desktop uploader</span>
        </div>
        <div class="app-tools">
          <Sheet v-model:open="settingsOpen">
            <SheetTrigger as-child>
              <Button
                variant="ghost"
                size="sm"
                :disabled="isStarting || uploadState.isUploading"
              >
                <SlidersHorizontal aria-hidden="true" /> Settings
              </Button>
            </SheetTrigger>
            <SheetContent
              side="right"
              class="settings-sheet w-full overflow-y-auto sm:max-w-md"
              aria-describedby="settings-description"
            >
              <div class="sheet-heading">
                <div>
                  <SheetTitle class="panel-title">
                    Upload settings
                  </SheetTitle>
                  <SheetDescription id="settings-description">
                    Changes are saved as you go.
                  </SheetDescription>
                </div>
                <SheetClose as-child>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="Close settings"
                  >
                    <X aria-hidden="true" />
                  </Button>
                </SheetClose>
              </div>
              <TooltipProvider disable-hoverable-content>
                <SettingsPanel />
              </TooltipProvider>
            </SheetContent>
          </Sheet>
          <Button
            class="appearance-button"
            variant="ghost"
            size="icon"
            :aria-label="colorMode === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'"
            :title="colorMode === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'"
            @click="colorMode = colorMode === 'dark' ? 'light' : 'dark'"
          >
            <Sun
              v-if="colorMode === 'dark'"
              aria-hidden="true"
            />
            <Moon
              v-else
              aria-hidden="true"
            />
          </Button>
        </div>
      </div>
    </header>

    <main
      class="workspace"
      :class="{ 'receiving-files': isDraggingFiles && canReceiveDrop && pendingFiles.length }"
    >
      <div class="workspace-heading">
        <h1
          ref="workspaceHeading"
          tabindex="-1"
        >
          {{ uploadState.isUploading ? 'Upload in progress' : 'Upload workspace' }}
        </h1>
        <p v-if="!uploadState.isUploading">
          Review your selection before sending it to Google Photos.
        </p>
        <p v-else-if="hasAccount">
          Google Photos · {{ selectedOption }}
        </p>
      </div>
      <Upload v-if="uploadState.isUploading" />
      <template v-else>
        <UploadResults
          v-if="hasResults && !pendingFiles.length"
          :disabled="isStarting || isChoosing"
          @retry="stageIncompleteFiles($event, lastDestination)"
        />
        <div class="workspace-grid">
          <div class="source-column">
            <section
              class="source-surface"
              :class="{ 'has-selections': pendingFiles.length }"
              aria-labelledby="source-title"
            >
              <div class="source-copy">
                <h2 id="source-title">
                  Add photos &amp; videos
                </h2>
                <p>Drop files or folders here.</p>
                <div class="picker-actions">
                  <Button
                    :disabled="isStarting || isChoosing"
                    @click="choosePaths(false)"
                  >
                    <Files aria-hidden="true" /> Choose files
                  </Button>
                  <Button
                    class="source-secondary"
                    :disabled="isStarting || isChoosing"
                    @click="choosePaths(true)"
                  >
                    <FolderOpen aria-hidden="true" /> Choose folders
                  </Button>
                </div>
              </div>
              <figure class="archive-print">
                <img
                  :src="tetons"
                  alt=""
                  width="880"
                  height="713"
                  draggable="false"
                >
                <figcaption>Ansel Adams, 1942</figcaption>
              </figure>
            </section>
            <UploadSelection
              :paths="pendingFiles"
              :incomplete-count="allowIncompletePaths.size"
              :disabled="isStarting || isChoosing"
              @remove="removePath"
              @clear="clearSelection"
              @paste-skipped="pasteSkippedFiles"
            />
          </div>

          <aside
            class="panel destination-panel"
            aria-labelledby="destination-title"
          >
            <header class="panel-heading">
              <h2
                id="destination-title"
                class="panel-title"
              >
                Destination
              </h2>
            </header>
            <div class="destination-fields">
              <div>
                <p class="field-label">
                  Google account
                </p>
                <GoogleAccountSelect
                  v-if="options.length"
                  v-model="selectedOption"
                  :options="options"
                  :disabled="isStarting"
                  :removing-account="removingAccount"
                  @item-removed="removeCredentials"
                  @add="openAccountSetup"
                />
                <Button
                  v-else
                  id="google-account-control"
                  variant="outline"
                  class="w-full justify-start"
                  :disabled="isStarting"
                  @click="openAccountSetup"
                >
                  <Plus aria-hidden="true" /> Add Google account
                </Button>
              </div>
              <div
                v-if="tokenBindingEmail"
                class="binding-note"
              >
                <p>This credential needs a token binding key from the rooted Android device it was captured from.</p>
                <Button
                  variant="outline"
                  size="sm"
                  :disabled="isExtractingTokenBinding"
                  @click="addTokenBindingAliasFromADB"
                >
                  {{ isExtractingTokenBinding ? 'Reading ADB…' : 'Read from ADB' }}
                </Button>
              </div>
              <fieldset :disabled="isStarting">
                <legend class="field-label">
                  Save to
                </legend>
                <label
                  v-for="option in destinations"
                  :key="option.value"
                  class="destination-option"
                >
                  <input
                    v-model="destination"
                    type="radio"
                    name="destination"
                    :value="option.value"
                  >
                  <component
                    :is="option.icon"
                    aria-hidden="true"
                  />
                  <span>{{ option.label }}</span>
                </label>
              </fieldset>
              <div
                v-if="destination === 'album'"
                class="album-field"
              >
                <label
                  class="field-label"
                  for="album-input"
                >Album name or key</label>
                <input
                  id="album-input"
                  v-model="albumNameOrKey"
                  class="text-field"
                  :disabled="isStarting"
                >
              </div>
            </div>
            <div class="destination-action">
              <Button
                class="start-button"
                :disabled="!canUpload"
                :aria-describedby="startHint ? 'start-hint' : undefined"
                @click="confirmUpload"
              >
                {{ isStarting ? 'Starting…' : 'Start upload' }}
                <ArrowRight aria-hidden="true" />
              </Button>
              <p
                v-if="startHint"
                id="start-hint"
              >
                {{ startHint }}
              </p>
            </div>
          </aside>
        </div>
      </template>
    </main>

    <div
      v-if="isDraggingFiles && canReceiveDrop && !pendingFiles.length"
      class="drop-overlay"
    >
      <div class="drop-choices">
        <p class="drop-heading">
          Where should these go?
        </p>
        <div
          v-for="option in destinations"
          :key="option.value"
          class="drop-zone"
          data-file-drop-target
          :data-drop-zone="option.value"
        >
          <component
            :is="option.icon"
            aria-hidden="true"
          />
          <div><h2>{{ option.label }}</h2><p>{{ option.dropHint }}</p></div>
        </div>
      </div>
    </div>

    <GoogleAuthSetup
      v-model:open="isAccountSetupOpen"
      @account-added="refreshCredentials"
      @return-focus="focusAccountControl"
    />
    <Toaster
      position="bottom-center"
      :theme="colorMode === 'dark' ? 'dark' : 'light'"
      rich-colors
      close-button
      :visible-toasts="2"
      :toast-options="{ closeButtonAriaLabel: 'Dismiss notification' }"
    />
  </div>
</template>
