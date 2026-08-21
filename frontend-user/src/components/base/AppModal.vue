<script setup lang="ts">
/**
 * 确认弹窗，用于替代原生 confirm。
 * 所有危险操作（删除资产/解除关系）都必须经过它。
 */
import { onBeforeUnmount, watch } from 'vue'

import AppButton from './AppButton.vue'

const props = withDefaults(
  defineProps<{
    open: boolean
    title: string
    message?: string
    detail?: string
    confirmText?: string
    cancelText?: string
    danger?: boolean
    loading?: boolean
  }>(),
  {
    message: '',
    detail: '',
    confirmText: '确认',
    cancelText: '取消',
    danger: false,
    loading: false,
  },
)

const emit = defineEmits<{ confirm: []; cancel: [] }>()

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') emit('cancel')
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
    enter-active-class="transition-opacity duration-150"
    leave-active-class="transition-opacity duration-150"
    enter-from-class="opacity-0"
    leave-to-class="opacity-0"
  >
    <div
      v-if="open"
      class="fixed inset-0 z-50 grid place-items-center bg-void/70 p-4 backdrop-blur-sm"
      @click.self="emit('cancel')"
    >
      <div
        role="alertdialog"
        aria-modal="true"
        :aria-label="title"
        class="corner-cut panel w-full max-w-sm rounded-sm p-4 shadow-2xl"
      >
        <h3 class="mb-2 text-[14px]" :class="danger ? 'text-danger' : 'text-ink'">{{ title }}</h3>
        <p v-if="message" class="text-[12px] leading-relaxed text-ink-dim">{{ message }}</p>
        <p v-if="detail" class="mono mt-2 rounded-xs bg-elevated p-2 text-[11px] text-ink-mute">
          {{ detail }}
        </p>
        <slot />
        <div class="mt-4 flex justify-end gap-2">
          <AppButton variant="ghost" :disabled="loading" @click="emit('cancel')">
            {{ cancelText }}
          </AppButton>
          <AppButton
            :variant="danger ? 'danger' : 'primary'"
            :loading="loading"
            @click="emit('confirm')"
          >
            {{ confirmText }}
          </AppButton>
        </div>
      </div>
    </div>
  </Transition>
</template>
