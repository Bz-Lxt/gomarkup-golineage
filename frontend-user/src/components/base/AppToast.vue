<script setup lang="ts">
import { useToast } from '@/composables/useToast'
import { UI } from '@/utils/palette'

const { items, dismiss } = useToast()

const accents: Record<string, string> = {
  success: UI.signal,
  error: UI.danger,
  warn: UI.amber,
  info: UI.inkDim,
}
</script>

<template>
  <div class="pointer-events-none fixed top-14 left-1/2 z-[100] flex w-80 -translate-x-1/2 flex-col gap-2">
    <TransitionGroup
      enter-active-class="transition-all duration-200 ease-out"
      leave-active-class="transition-all duration-150 ease-in"
      enter-from-class="opacity-0 -translate-y-2"
      leave-to-class="opacity-0 -translate-y-2"
      move-class="transition-transform duration-200"
    >
      <div
        v-for="toast in items"
        :key="toast.id"
        role="status"
        class="corner-cut panel pointer-events-auto flex items-start gap-2 rounded-sm p-2.5 shadow-xl"
        :style="{ borderColor: `${accents[toast.kind]}55` }"
      >
        <span
          class="mt-1 size-1.5 shrink-0 rounded-full"
          :style="{ backgroundColor: accents[toast.kind] }"
          aria-hidden="true"
        />
        <div class="min-w-0 flex-1">
          <p class="text-[12px] leading-snug break-words" :style="{ color: accents[toast.kind] }">
            {{ toast.message }}
          </p>
          <p v-if="toast.traceId" class="mono mt-1 text-[10px] text-ink-mute">
            trace {{ toast.traceId.slice(0, 8) }}
          </p>
        </div>
        <button
          type="button"
          aria-label="关闭提示"
          class="grid size-4 shrink-0 place-items-center rounded-xs text-ink-mute transition-colors hover:text-ink"
          @click="dismiss(toast.id)"
        >
          <svg viewBox="0 0 10 10" class="size-2.5" fill="none" stroke="currentColor" stroke-width="1.5">
            <path d="M1.5 1.5L8.5 8.5M8.5 1.5L1.5 8.5" stroke-linecap="round" />
          </svg>
        </button>
      </div>
    </TransitionGroup>
  </div>
</template>
