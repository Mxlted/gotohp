<script setup lang="ts">
import { ref, watch } from 'vue'
import { Browser } from '@wailsio/runtime'
import { toast } from 'vue-sonner'
import { ChevronDown, ExternalLink, X } from '@lucide/vue'
import { ConfigManager } from '../../bindings/app/core'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetTitle,
} from '@/components/ui/sheet'

const embeddedSetupURL = 'https://accounts.google.com/EmbeddedSetup'

const emit = defineEmits<{
  (event: 'account-added'): void
  (event: 'return-focus'): void
}>()

const isOpen = defineModel<boolean>('open', { default: false })
const oauthToken = ref('')
const rawCredential = ref('')
const isConnecting = ref(false)
const isAddingRawCredential = ref(false)
const showAdvanced = ref(false)

watch(isOpen, (open) => {
  if (!open) {
    oauthToken.value = ''
    rawCredential.value = ''
    showAdvanced.value = false
  }
})

async function openEmbeddedSetup() {
  try {
    await Browser.OpenURL(embeddedSetupURL)
  } catch (error) {
    toast.error('Could not open the system browser', {
      description: error instanceof Error ? error.message : String(error),
    })
  }
}

async function connectAccount() {
  const normalizedToken = oauthToken.value.trim()
  if (!normalizedToken) return

  isConnecting.value = true
  try {
    const connectedEmail = await ConfigManager.AddGoogleAccount(normalizedToken)
    oauthToken.value = ''
    isOpen.value = false
    emit('account-added')
    toast.success('Google account connected.', {
      description: connectedEmail,
    })
  } catch (error) {
    toast.error('Could not connect Google account', {
      description: error instanceof Error ? error.message : String(error),
    })
  } finally {
    oauthToken.value = ''
    isConnecting.value = false
  }
}

async function addRawCredential() {
  const credential = rawCredential.value.trim()
  if (!credential) return

  isAddingRawCredential.value = true
  try {
    await ConfigManager.AddCredentials(credential)
    rawCredential.value = ''
    isOpen.value = false
    emit('account-added')
    toast.success('Credentials added.')
  } catch (error) {
    toast.error('Could not add credentials', {
      description: error instanceof Error ? error.message : String(error),
    })
  } finally {
    isAddingRawCredential.value = false
  }
}
</script>

<template>
  <Sheet v-model:open="isOpen">
    <SheetContent
      side="right"
      class="settings-sheet w-full overflow-y-auto sm:max-w-md"
      :aria-describedby="undefined"
      style="--wails-draggable: none"
      @close-auto-focus.prevent="emit('return-focus')"
    >
      <div class="sheet-heading">
        <SheetTitle class="panel-title">
          Connect Google Photos
        </SheetTitle>
        <SheetClose as-child>
          <Button
            variant="ghost"
            size="icon"
            aria-label="Close account setup"
          >
            <X aria-hidden="true" />
          </Button>
        </SheetClose>
      </div>
      <div class="flex flex-col p-5">
        <div class="flex flex-col gap-4">
          <!-- Step 1: open sign-in -->
          <div class="flex flex-col gap-2">
            <p class="flex items-center gap-2 text-sm font-medium select-none">
              <span class="flex size-5 shrink-0 items-center justify-center rounded-full bg-muted text-xs">1</span>
              Open the Google sign-in page
            </p>
            <Button
              type="button"
              variant="outline"
              class="cursor-pointer select-none"
              :disabled="isConnecting"
              @click="openEmbeddedSetup"
            >
              <ExternalLink class="size-4" />
              Open sign-in
            </Button>
            <ol class="list-decimal space-y-1 rounded-lg border bg-muted/30 py-2.5 pr-3 pl-7 text-xs leading-snug text-muted-foreground">
              <li>
                Sign in, then click <span class="text-foreground">I agree</span>. The page may hang on a spinner. That's fine.
              </li>
              <li>Open DevTools, then Application or Storage → Cookies → accounts.google.com.</li>
              <li>Copy the <code class="text-foreground">oauth_token</code> cookie's value.</li>
            </ol>
          </div>

          <!-- Step 2: paste token -->
          <div class="flex flex-col gap-2">
            <Label
              for="google-oauth-token"
              class="flex items-center gap-2 text-sm font-medium"
            >
              <span class="flex size-5 shrink-0 items-center justify-center rounded-full bg-muted text-xs">2</span>
              Paste the oauth_token value
            </Label>
            <Input
              id="google-oauth-token"
              v-model="oauthToken"
              type="password"
              autocomplete="off"
              spellcheck="false"
              placeholder="Paste the cookie value"
              :disabled="isConnecting"
              @keydown.enter="connectAccount"
            />
            <Button
              type="button"
              class="cursor-pointer select-none"
              :disabled="!oauthToken.trim() || isConnecting"
              @click="connectAccount"
            >
              {{ isConnecting ? 'Connecting...' : 'Connect account' }}
            </Button>
          </div>
        </div>

        <div class="mt-4 border-t pt-3">
          <button
            type="button"
            class="flex w-full items-center gap-1 text-xs text-muted-foreground select-none hover:text-foreground"
            @click="showAdvanced = !showAdvanced"
          >
            <ChevronDown
              class="size-3.5 transition-transform"
              :class="showAdvanced && 'rotate-180'"
            />
            Paste a captured credential instead
          </button>
          <div
            v-if="showAdvanced"
            class="mt-3 flex flex-col gap-2"
          >
            <Input
              id="google-raw-credential"
              v-model="rawCredential"
              type="password"
              autocomplete="off"
              spellcheck="false"
              placeholder="androidId=...&Email=..."
              :disabled="isAddingRawCredential"
              @keydown.enter="addRawCredential"
            />
            <Button
              type="button"
              variant="outline"
              class="cursor-pointer select-none"
              :disabled="!rawCredential.trim() || isAddingRawCredential"
              @click="addRawCredential"
            >
              {{ isAddingRawCredential ? 'Adding...' : 'Add credential' }}
            </Button>
          </div>
        </div>
      </div>
    </SheetContent>
  </Sheet>
</template>
