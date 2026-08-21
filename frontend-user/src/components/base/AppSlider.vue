<script setup lang="ts">
/**
 * 时间轴滑块。
 *
 * 原生 range 在深色主题下几乎无法定制（各浏览器伪元素不统一），
 * 因此自绘轨道与滑块，同时保留完整键盘支持。
 */
import { computed, ref } from 'vue'

const props = withDefaults(
  defineProps<{
    modelValue: number
    min: number
    max: number
    step?: number
    disabled?: boolean
    /** 历史模式下整体转紫，与「当前态」形成颜色区分。 */
    accent?: string
    ariaLabel?: string
  }>(),
  { step: 1, disabled: false, accent: '#A78BFA', ariaLabel: '时间轴' },
)

const emit = defineEmits<{
  'update:modelValue': [number]
  /** 拖动结束才触发昂贵的快照请求，拖动过程只更新本地值。 */
  commit: [number]
}>()

const track = ref<HTMLElement | null>(null)
const dragging = ref(false)

const percent = computed(() => {
  const span = props.max - props.min
  if (span <= 0) return 100
  return ((props.modelValue - props.min) / span) * 100
})

function valueFromClientX(clientX: number): number {
  if (!track.value) return props.modelValue
  const rect = track.value.getBoundingClientRect()
  const ratio = Math.min(1, Math.max(0, (clientX - rect.left) / rect.width))
  const raw = props.min + ratio * (props.max - props.min)
  return Math.round(raw / props.step) * props.step
}

function onPointerDown(event: PointerEvent) {
  if (props.disabled) return
  dragging.value = true
  ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
  emit('update:modelValue', valueFromClientX(event.clientX))
}

function onPointerMove(event: PointerEvent) {
  if (!dragging.value || props.disabled) return
  emit('update:modelValue', valueFromClientX(event.clientX))
}

function onPointerUp(event: PointerEvent) {
  if (!dragging.value) return
  dragging.value = false
  ;(event.currentTarget as HTMLElement).releasePointerCapture(event.pointerId)
  emit('commit', valueFromClientX(event.clientX))
}

function onKeydown(event: KeyboardEvent) {
  if (props.disabled) return
  const span = props.max - props.min
  const big = Math.max(props.step, Math.round(span / 20))
  let next: number | null = null
  switch (event.key) {
    case 'ArrowLeft':
    case 'ArrowDown':
      next = props.modelValue - props.step
      break
    case 'ArrowRight':
    case 'ArrowUp':
      next = props.modelValue + props.step
      break
    case 'PageDown':
      next = props.modelValue - big
      break
    case 'PageUp':
      next = props.modelValue + big
      break
    case 'Home':
      next = props.min
      break
    case 'End':
      next = props.max
      break
    default:
      return
  }
  event.preventDefault()
  const clamped = Math.min(props.max, Math.max(props.min, next))
  emit('update:modelValue', clamped)
  emit('commit', clamped)
}
</script>

<template>
  <div
    ref="track"
    role="slider"
    tabindex="0"
    :aria-label="ariaLabel"
    :aria-valuemin="min"
    :aria-valuemax="max"
    :aria-valuenow="modelValue"
    :aria-disabled="disabled"
    class="relative h-5 cursor-pointer touch-none select-none"
    :class="disabled && 'cursor-not-allowed opacity-40'"
    @pointerdown="onPointerDown"
    @pointermove="onPointerMove"
    @pointerup="onPointerUp"
    @pointercancel="onPointerUp"
    @keydown="onKeydown"
  >
    <div class="absolute inset-x-0 top-1/2 h-px -translate-y-1/2 bg-line-strong" />
    <div
      class="absolute top-1/2 left-0 h-px -translate-y-1/2 transition-[width] duration-75"
      :style="{ width: `${percent}%`, backgroundColor: accent }"
    />
    <div
      class="absolute top-1/2 size-3 -translate-x-1/2 -translate-y-1/2 rotate-45 border transition-shadow"
      :style="{
        left: `${percent}%`,
        backgroundColor: accent,
        borderColor: accent,
        boxShadow: dragging ? `0 0 12px ${accent}` : 'none',
      }"
    />
    <slot name="bubble" :percent="percent" :dragging="dragging" />
  </div>
</template>
