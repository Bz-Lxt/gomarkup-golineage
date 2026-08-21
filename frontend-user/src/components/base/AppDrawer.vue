<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'

const props = withDefaults(
  defineProps<{ open: boolean; title?: string; subtitle?: string; width?: number }>(),
  { title: '', subtitle: '', width: 360 },
)

const emit = defineEmits<{ close: [] }>()

const panel = ref<HTMLElement | null>(null)

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    emit('close')
    return
  }
  // 焦点陷阱：抽屉打开时 Tab 不应跑到背后的画布上。
  if (event.key !== 'Tab' || !panel.value) return
  const focusable = panel.value.querySelectorAll<HTMLElement>(
    'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
  )
  if (focusable.length === 0) return
  const first = focusable[0]
  const last = focusable[focusable.length - 1]
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first.focus()
  }
}

watch(
  () => props.open,
  (open) => {
    if (open) window.addEventListener('keydown', onKeydown)
    else window.removeEventListener('keydown', onKeydown)
  },
)

onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <Transition
    enter-active-class="transition-transform duration-240 ease-[cubic-bezier(.32,.72,0,1)]"
    leave-active-class="transition-transform duration-200 ease-[cubic-bezier(.32,.72,0,1)]"
    enter-from-class="translate-x-full"
    leave-to-class="translate-x-full"
  >
    <aside
      v-if="open"
      ref="panel"
      role="dialog"
      aria-modal="false"
      :aria-label="title"
      class="corner-cut panel relative z-30 flex h-full shrink-0 flex-col border-l max-md:absolute max-md:inset-y-0 max-md:right-0 max-md:w-full"
      :style="{ width: `${width}px` }"
    >
      <header class="flex h-11 shrink-0 items-center gap-2 border-b border-line px-3">
        <div class="min-w-0 flex-1">
          <h2 class="truncate text-[13px] leading-tight">{{ title }}</h2>
          <p v-if="subtitle" class="mono truncate text-[10px] text-ink-mute">{{ subtitle }}</p>
        </div>
        <button
          type="button"
          aria-label="关闭"
          class="grid size-6 shrink-0 place-items-center rounded-xs text-ink-mute transition-colors hover:bg-elevated hover:text-ink"
          @click="emit('close')"
        >
          <svg viewBox="0 0 12 12" class="size-3" fill="none" stroke="currentColor" stroke-width="1.5">
            <path d="M2 2L10 10M10 2L2 10" stroke-linecap="round" />
          </svg>
        </button>
      </header>

      <div class="min-h-0 flex-1 overflow-y-auto">
        <slot />
      </div>

      <footer v-if="$slots.footer" class="shrink-0 border-t border-line p-3">
        <slot name="footer" />
      </footer>
    </aside>
  </Transition>
</template>
