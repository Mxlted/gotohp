<script setup lang="ts">
import { Check, ChevronDown, ChevronRight, Plus, UserRound, UserRoundMinus } from '@lucide/vue'
import {
  DropdownMenuContent, DropdownMenuItem, DropdownMenuItemIndicator, DropdownMenuPortal,
  DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuRoot, DropdownMenuSeparator,
  DropdownMenuSub, DropdownMenuSubContent, DropdownMenuSubTrigger, DropdownMenuTrigger,
} from 'reka-ui'
import Button from './ui/button/Button.vue'

defineProps<{ options: string[], removingAccount: string, disabled: boolean }>()
const emit = defineEmits<{ 'item-removed': [email: string], add: [] }>()
const account = defineModel<string>({ required: true })
</script>

<template>
  <DropdownMenuRoot>
    <DropdownMenuTrigger
      :disabled="disabled"
      as-child
    >
      <Button
        id="google-account-control"
        variant="outline"
        class="w-full min-w-0 justify-start"
        :aria-label="account ? 'Google account, ' + account : 'Select Google account'"
        :title="account"
      >
        <UserRound aria-hidden="true" />
        <span class="min-w-0 flex-1 truncate text-left">{{ account || 'Select account' }}</span>
        <ChevronDown
          aria-hidden="true"
          class="text-muted-foreground"
        />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuPortal>
      <DropdownMenuContent
        class="account-menu"
        align="start"
        :side-offset="5"
        :collision-padding="8"
      >
        <DropdownMenuRadioGroup
          v-model="account"
          aria-label="Google accounts"
        >
          <DropdownMenuRadioItem
            v-for="option in options"
            :key="option"
            :value="option"
            class="account-item"
            :title="option"
          >
            <span class="account-check"><DropdownMenuItemIndicator><Check aria-hidden="true" /></DropdownMenuItemIndicator></span>
            <span class="truncate">{{ option }}</span>
          </DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
        <DropdownMenuSeparator class="my-1 border-t" />
        <DropdownMenuItem
          class="account-item"
          @select="emit('add')"
        >
          <Plus aria-hidden="true" /> Add account
        </DropdownMenuItem>
        <DropdownMenuSub>
          <DropdownMenuSubTrigger class="account-item">
            <UserRoundMinus aria-hidden="true" /> Remove account <ChevronRight
              aria-hidden="true"
              class="ml-auto"
            />
          </DropdownMenuSubTrigger>
          <DropdownMenuPortal>
            <DropdownMenuSubContent
              class="account-menu"
              :side-offset="3"
              :collision-padding="8"
              aria-label="Remove account"
            >
              <DropdownMenuItem
                v-for="option in options"
                :key="option"
                class="account-item"
                :title="option"
                :aria-label="'Remove ' + option"
                :disabled="removingAccount === option"
                @select="emit('item-removed', option)"
              >
                <span class="truncate">{{ option }}</span>
              </DropdownMenuItem>
            </DropdownMenuSubContent>
          </DropdownMenuPortal>
        </DropdownMenuSub>
      </DropdownMenuContent>
    </DropdownMenuPortal>
  </DropdownMenuRoot>
</template>

<style scoped>
.account-menu {
  z-index: 50;
  width: max-content;
  min-width: min(var(--reka-dropdown-menu-trigger-width, 12rem), calc(100vw - 16px));
  max-width: min(24rem, calc(100vw - 16px));
  max-height: var(--reka-dropdown-menu-content-available-height, 70vh);
  overflow-y: auto;
  padding: 4px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--card);
  box-shadow: 0 4px 14px #171b1718;
}
.account-item { display: flex; align-items: center; gap: 8px; min-height: 33px; border-radius: 4px; padding: 5px 9px; outline: none; font-size: 13px; cursor: pointer; }
.account-item[data-highlighted], .account-item[data-state=open] { background: var(--accent); }
.account-item[data-disabled] { opacity: .5; pointer-events: none; }
.account-check, .account-item svg { width: 15px; height: 15px; flex: none; }
</style>
