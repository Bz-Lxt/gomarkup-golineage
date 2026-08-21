<script setup lang="ts">
withDefaults(defineProps<{ size?: number; label?: string }>(), { size: 16, label: '' })
</script>

<template>
  <div class="inline-flex items-center gap-2" role="status" :aria-label="label || '加载中'">
    <!-- 扫描弧而非通用圆环：与控制台的雷达语汇一致。 -->
    <svg
      class="anim-spin-arc"
      :width="size"
      :height="size"
      viewBox="0 0 24 24"
      fill="none"
      aria-hidden="true"
    >
      <circle cx="12" cy="12" r="9" stroke="var(--color-line-strong)" stroke-width="2" />
      <path
        d="M21 12a9 9 0 0 0-9-9"
        stroke="var(--color-signal)"
        stroke-width="2"
        stroke-linecap="round"
      />
    </svg>
    <span v-if="label" class="text-[11px] text-ink-mute">{{ label }}</span>
  </div>
</template>
