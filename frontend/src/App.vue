<script setup lang="ts">
import { Label } from '@/components/ui/label'
import { TooltipProvider } from '@/components/ui/tooltip'
import {
  Sheet,
  SheetContent,
  SheetTrigger,
} from '@/components/ui/sheet'
import { useColorMode } from '@vueuse/core'
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { Files, FolderOpen } from '@lucide/vue'
import { ConfigManager } from '../bindings/app/core'
import { Dialogs, Events } from '@wailsio/runtime'
import Button from "./components/ui/button/Button.vue"
import GoogleAccountSelect from './components/GoogleAccountSelect.vue'
import GoogleAuthSetup from "./components/GoogleAuthSetup.vue"
import './index.css'
import SettingsPanel from "./SettingsPanel.vue"
import Upload from './Upload.vue'
import UploadSelection from './components/UploadSelection.vue'
import { uploadManager } from './utils/UploadManager'
import Toaster from './components/ui/sonner/Sonner.vue'

import { toast } from "vue-sonner"

useColorMode().value = "dark"

const { state: uploadState } = uploadManager
const emptySelection = computed(() => uploadState.results.warnings.find(warning => warning.code === 'no-matching-files'))
const copyButtonText = ref('Copy as JSON')
let copyReset: ReturnType<typeof setTimeout> | undefined
let showedUploadError = false

watch(() => uploadState.isUploading, running => {
  if (running) {
    showedUploadError = false
    toast.dismiss('upload-error')
  }
}, { flush: 'sync' })

// Drag state for dual drop zones
const isDraggingFiles = ref(false)

const pendingFiles = ref<string[]>([])
const destination = ref('regular')
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
  !isStarting.value && !isChoosing.value && (destination.value !== 'album' || !!albumNameOrKey.value.trim()))

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

onMounted(async () => {
  await refreshCredentials()

})

async function handleCopyClick() {
  clearTimeout(copyReset)
  if (await uploadManager.copyResultsAsJson()) {
    copyButtonText.value = 'Copied!'
    copyReset = setTimeout(() => { copyButtonText.value = 'Copy as JSON' }, 1000)
  } else {
    copyButtonText.value = 'Copy as JSON'
    toast.error('Could not copy upload results')
  }
}
// Global drag event handlers to detect file dragging
let dragLeaveTimeout: ReturnType<typeof setTimeout> | null = null

const onDragEnter = (e: DragEvent) => {
  if (!e.dataTransfer?.types.includes('Files')) return
  
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
  setTimeout(() => {
    isDraggingFiles.value = false
  }, 100)
}

function stageFiles(files: string[], dropZone = 'regular') {
  if (!files.length || isStarting.value || uploadState.isUploading) return
  if (!pendingFiles.value.length) {
    destination.value = dropZone === 'album' || dropZone === 'auto-album' ? dropZone : 'regular'
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
  destination.value = 'regular'
  albumNameOrKey.value = ''
}

function removePath(path: string) {
  pendingFiles.value = pendingFiles.value.filter(item => item !== path)
  if (!pendingFiles.value.length) clearSelection()
}

async function confirmUpload() {
  if (!canUpload.value) return
  isStarting.value = true
  try {
    const cancelled = await Events.Emit('startUpload', {
      files: [...pendingFiles.value],
      destination: {
        account: selectedOption.value,
        albumName: destination.value === 'album' ? albumNameOrKey.value.trim() : '',
        autoAlbum: destination.value === 'auto-album',
      },
    })
    if (cancelled) {
      toast.error('Upload request was cancelled')
      return
    }
    clearSelection()
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
    stageFiles(data.files, data.dropZone)
  })
})

onUnmounted(() => {
  stopFileDrops?.()
  clearTimeout(copyReset)
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
  <main
    class="w-screen h-screen flex flex-col items-center"
    style="--wails-draggable: drag"
  >
    <!-- Drop zones shown when dragging files -->
    <div
      v-if="!uploadState.isUploading && isDraggingFiles && !pendingFiles.length"
      class="w-screen h-screen flex flex-col gap-3 p-6"
      style="--wails-draggable: none"
    >
      <div
        data-file-drop-target
        data-drop-zone="regular"
        class="flex-1 flex flex-col items-center justify-center border-2 border-dashed border-muted-foreground/50 rounded-xl transition-all duration-200 drop-zone"
      >
        <h2 class="text-xl font-semibold select-none text-muted-foreground">
          Upload Only
        </h2>
        <p class="text-sm text-muted-foreground/70 mt-2 select-none text-center px-4">
          Upload files without adding to any album
        </p>
      </div>
      <div
        data-file-drop-target
        data-drop-zone="album"
        class="flex-1 flex flex-col items-center justify-center border-2 border-dashed border-muted-foreground/50 rounded-xl transition-all duration-200 drop-zone"
      >
        <h2 class="text-xl font-semibold select-none text-muted-foreground">
          Upload to Album
        </h2>
        <p class="text-sm text-muted-foreground/70 mt-2 select-none text-center px-4">
          Upload and add to a specific album (you'll enter the name)
        </p>
      </div>
      <div
        data-file-drop-target
        data-drop-zone="auto-album"
        class="flex-1 flex flex-col items-center justify-center border-2 border-dashed border-muted-foreground/50 rounded-xl transition-all duration-200 drop-zone"
      >
        <h2 class="text-xl font-semibold select-none text-muted-foreground">
          Auto Album
        </h2>
        <p class="text-sm text-muted-foreground/70 mt-2 select-none text-center px-4">
          Upload and create albums automatically based on folder names
        </p>
      </div>
    </div>

    <section
      v-else-if="!uploadState.isUploading"
      class="w-full h-full overflow-y-auto"
      data-file-drop-target
      style="--wails-draggable: none"
    >
      <div
        class="mx-auto flex w-full max-w-md flex-col gap-4 px-6 py-6"
        :class="{ 'pt-14': !pendingFiles.length }"
      >
        <UploadSelection
          v-if="pendingFiles.length"
          v-model:destination="destination"
          v-model:album="albumNameOrKey"
          :paths="pendingFiles"
          :disabled="isStarting || isChoosing"
          @remove="removePath"
        />
        <div
          v-else
          class="text-center"
        >
          <h1 class="text-xl font-semibold">
            Choose photos and videos
          </h1>
          <p class="mt-1 text-sm text-muted-foreground">
            Drop files or folders here to review them before uploading.
          </p>
        </div>

        <div class="flex justify-center gap-2">
          <Button
            variant="outline"
            :disabled="isStarting || isChoosing"
            @click="choosePaths(false)"
          >
            <Files aria-hidden="true" /> Choose files
          </Button>
          <Button
            variant="outline"
            :disabled="isStarting || isChoosing"
            @click="choosePaths(true)"
          >
            <FolderOpen aria-hidden="true" /> Choose folders
          </Button>
        </div>

        <div class="flex flex-wrap items-center justify-center gap-2">
          <GoogleAccountSelect
            v-if="options.length"
            v-model="selectedOption"
            :options="options"
            :removing-account="removingAccount"
            @item-removed="removeCredentials"
            @add="openAccountSetup"
          />
          <Button
            v-else
            variant="outline"
            @click="openAccountSetup"
          >
            Add Google account
          </Button>
          <Sheet>
            <SheetTrigger as-child>
              <Button
                variant="outline"
                :disabled="isStarting"
              >
                Settings
              </Button>
            </SheetTrigger>
            <SheetContent side="bottom">
              <TooltipProvider disable-hoverable-content>
                <SettingsPanel />
              </TooltipProvider>
            </SheetContent>
          </Sheet>
        </div>
        <div
          v-if="tokenBindingEmail"
          class="w-full rounded-lg border p-3 flex flex-col gap-3"
        >
          <p class="text-sm text-muted-foreground">
            This credential needs a token binding key from the rooted Android device it was captured from.
          </p>
          <Button
            :disabled="isExtractingTokenBinding"
            @click="addTokenBindingAliasFromADB"
          >
            {{ isExtractingTokenBinding ? 'Reading ADB...' : 'Read from ADB' }}
          </Button>
        </div>

        <template v-if="pendingFiles.length">
          <p
            v-if="!hasAccount"
            class="text-center text-sm text-muted-foreground"
          >
            Select a Google account to start this upload.
          </p>
          <div class="flex justify-end gap-2 border-t pt-4">
            <Button
              variant="outline"
              :disabled="isStarting || isChoosing"
              @click="clearSelection"
            >
              Cancel
            </Button>
            <Button
              :disabled="!canUpload"
              @click="confirmUpload"
            >
              {{ isStarting ? 'Starting…' : 'Start upload' }}
            </Button>
          </div>
        </template>

        <div
          v-else-if="uploadState.uploadedFiles > 0 || uploadState.results.fail.length > 0 || uploadState.results.warnings.length > 0"
          class="flex flex-col items-center gap-2 rounded-lg border p-4"
        >
          <h2 class="font-semibold">
            Upload results
          </h2>
          <p
            v-if="emptySelection"
            class="text-center text-sm text-muted-foreground"
          >
            {{ emptySelection.reason }}
          </p>
          <Label class="text-muted-foreground">Successful: {{ uploadState.results.success.length }}</Label>
          <Label class="text-muted-foreground">Failed: {{ uploadState.results.fail.length }}</Label>
          <Label class="text-muted-foreground">Skipped: {{ uploadState.results.skipped.length }}</Label>
          <Label class="text-muted-foreground">Warnings: {{ uploadState.results.warnings.length }}</Label>
          <Button
            variant="outline"
            class="min-w-[125px]"
            @click="handleCopyClick"
          >
            {{ copyButtonText }}
          </Button>
        </div>
      </div>
    </section>
    <div
      v-if="uploadState.isUploading"
      class="w-full h-full"
    >
      <Upload />
    </div>
    <GoogleAuthSetup
      v-model:open="isAccountSetupOpen"
      @account-added="refreshCredentials"
    />
    <Toaster
      position="bottom-center"
      rich-colors
      close-button
      :visible-toasts="2"
      :toast-options="{ closeButtonAriaLabel: 'Dismiss notification' }"
    />
  </main>
</template>
