<script setup lang="ts">
withDefaults(
  defineProps<{
    variant?: 'primary' | 'ghost' | 'danger' | 'subtle'
    size?: 'sm' | 'md'
    disabled?: boolean
    loading?: boolean
    title?: string
    block?: boolean
  }>(),
  { variant: 'ghost', size: 'md', disabled: false, loading: false, title: '', block: false },
)

defineEmits<{ click: [MouseEvent] }>()
</script>

<template>
  <button
    type="button"
    :title="title"
    :disabled="disabled || loading"
    :class="[
      'inline-flex items-center justify-center gap-1.5 rounded-xs border font-medium',
      'transition-colors duration-150 select-none whitespace-nowrap',
      'disabled:cursor-not-allowed disabled:opacity-40',
      size === 'sm' ? 'h-6 px-2 text-[11px]' : 'h-7 px-3 text-[12px]',
      block && 'w-full',
      variant === 'primary' &&
        'border-signal/60 bg-signal/15 text-signal hover:bg-signal/25 hover:border-signal',
      variant === 'ghost' &&
        'border-line bg-elevated text-ink-dim hover:text-ink hover:border-line-strong',
      variant === 'subtle' &&
        'border-transparent bg-transparent text-ink-mute hover:text-ink hover:bg-elevated',
      variant === 'danger' &&
        'border-danger/50 bg-danger/10 text-danger hover:bg-danger/20 hover:border-danger',
    ]"
    @click="$emit('click', $event)"
  >
    <span
      v-if="loading"
      class="anim-spin-arc size-3 rounded-full border border-current border-t-transparent"
      aria-hidden="true"
    />
    <slot />
  </button>
</template>
