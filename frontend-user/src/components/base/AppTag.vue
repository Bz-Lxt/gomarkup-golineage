<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    color?: string
    /** solid 用于类型标识，outline 用于状态标识。 */
    variant?: 'solid' | 'outline'
    size?: 'xs' | 'sm'
    dot?: boolean
  }>(),
  { color: '#8B95A8', variant: 'outline', size: 'xs', dot: false },
)

const style = computed(() =>
  props.variant === 'solid'
    ? { color: props.color, backgroundColor: `${props.color}22`, borderColor: `${props.color}55` }
    : { color: props.color, borderColor: `${props.color}44` },
)
</script>

<template>
  <span
    class="inline-flex items-center gap-1 rounded-xs border font-medium whitespace-nowrap"
    :class="size === 'xs' ? 'h-[18px] px-1.5 text-[10px]' : 'h-5 px-2 text-[11px]'"
    :style="style"
  >
    <span
      v-if="dot"
      class="size-1.5 shrink-0 rounded-full"
      :style="{ backgroundColor: color }"
      aria-hidden="true"
    />
    <slot />
  </span>
</template>
