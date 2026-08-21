<script setup lang="ts">
withDefaults(
  defineProps<{
    modelValue: string
    placeholder?: string
    label?: string
    error?: string
    disabled?: boolean
    mono?: boolean
    type?: string
  }>(),
  { placeholder: '', label: '', error: '', disabled: false, mono: false, type: 'text' },
)

const emit = defineEmits<{ 'update:modelValue': [string]; enter: [] }>()

function onInput(event: Event) {
  emit('update:modelValue', (event.target as HTMLInputElement).value)
}
</script>

<template>
  <label class="block">
    <span v-if="label" class="label-caps mb-1 block">{{ label }}</span>
    <input
      :value="modelValue"
      :type="type"
      :placeholder="placeholder"
      :disabled="disabled"
      :aria-invalid="!!error"
      :class="[
        'h-7 w-full rounded-xs border bg-elevated px-2 text-[12px] text-ink',
        'placeholder:text-ink-mute transition-colors duration-150',
        'focus:outline-none disabled:opacity-40',
        mono && 'mono',
        error
          ? 'border-danger focus:border-danger'
          : 'border-line focus:border-signal focus:shadow-[0_0_16px_-6px_var(--color-signal)]',
      ]"
      @input="onInput"
      @keyup.enter="$emit('enter')"
    />
    <span v-if="error" class="mt-1 block text-[11px] text-danger">{{ error }}</span>
  </label>
</template>
