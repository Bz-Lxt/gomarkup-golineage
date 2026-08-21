<script setup lang="ts">
/**
 * 原生 select 的箭头在各平台渲染差异极大，全局 CSS 已重置为自绘 SVG。
 * 这里只负责外观与错误态。
 */
withDefaults(
  defineProps<{
    modelValue: string
    options: Array<{ value: string; label: string }>
    label?: string
    error?: string
    disabled?: boolean
    placeholder?: string
  }>(),
  { label: '', error: '', disabled: false, placeholder: '' },
)

const emit = defineEmits<{ 'update:modelValue': [string] }>()

function onChange(event: Event) {
  emit('update:modelValue', (event.target as HTMLSelectElement).value)
}
</script>

<template>
  <label class="block">
    <span v-if="label" class="label-caps mb-1 block">{{ label }}</span>
    <select
      :value="modelValue"
      :disabled="disabled"
      :aria-invalid="!!error"
      :class="[
        'h-7 w-full rounded-xs border bg-elevated pl-2 text-[12px] text-ink',
        'transition-colors duration-150 focus:outline-none disabled:opacity-40',
        error ? 'border-danger' : 'border-line focus:border-signal',
      ]"
      @change="onChange"
    >
      <option v-if="placeholder" value="">{{ placeholder }}</option>
      <option v-for="opt in options" :key="opt.value" :value="opt.value">
        {{ opt.label }}
      </option>
    </select>
    <span v-if="error" class="mt-1 block text-[11px] text-danger">{{ error }}</span>
  </label>
</template>
