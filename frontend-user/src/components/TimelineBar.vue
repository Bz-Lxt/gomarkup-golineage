<script setup lang="ts">
import { computed } from 'vue'

import AppButton from '@/components/base/AppButton.vue'
import AppSlider from '@/components/base/AppSlider.vue'
import AppSpinner from '@/components/base/AppSpinner.vue'
import { useTimelineStore } from '@/stores/timeline'
import { formatDateTime, formatShort } from '@/utils/format'
import { UI } from '@/utils/palette'

const timeline = useTimelineStore()

const accent = computed(() => (timeline.historyMode ? UI.violet : UI.signal))
const ticks = computed(() => {
  const min = timeline.minTime
  const max = timeline.maxTime
  const span = max - min
  if (span <= 0) return []
  return [0, 0.25, 0.5, 0.75, 1].map((p) => ({
    p,
    label: formatShort(min + span * p),
  }))
})

async function onCommit(value: number) {
  const nearLive = timeline.maxTime - value < 2000
  if (nearLive) {
    await timeline.returnToLive()
    return
  }
  await timeline.seek(value)
}
</script>

<template>
  <footer
    class="panel shrink-0 border-t px-4 py-2"
    :class="timeline.historyMode ? 'glow-violet' : ''"
  >
    <div class="mb-1 flex items-center gap-3">
      <p class="label-caps" :class="timeline.historyMode ? '!text-violet' : ''">
        {{ timeline.historyMode ? '历史回溯' : '时间轴' }}
      </p>
      <span class="mono text-[11px]" :style="{ color: accent }">
        {{ formatDateTime(timeline.cursor) }}
      </span>
      <span v-if="timeline.snapshotMeta" class="mono text-[10px] text-ink-mute">
        重放 {{ timeline.snapshotMeta.events_applied }} 条 · {{ timeline.snapshotMeta.duration_ms }}ms
      </span>
      <AppSpinner v-if="timeline.snapshotLoading" :size="12" label="重放中" />
      <div class="ml-auto flex items-center gap-1.5">
        <AppButton
          v-if="timeline.historyMode"
          size="sm"
          :loading="timeline.diffLoading"
          @click="timeline.compareWithNow()"
        >
          Diff 当前
        </AppButton>
        <AppButton
          v-if="timeline.historyMode"
          size="sm"
          variant="primary"
          @click="timeline.returnToLive()"
        >
          回到当前
        </AppButton>
      </div>
    </div>

    <AppSlider
      :model-value="timeline.cursor"
      :min="timeline.minTime"
      :max="timeline.maxTime"
      :step="60_000"
      :accent="accent"
      aria-label="血缘时间轴"
      @update:model-value="timeline.cursor = $event"
      @commit="onCommit"
    />

    <div class="mt-1 hidden justify-between text-[10px] text-ink-mute sm:flex">
      <span v-for="t in ticks" :key="t.p" class="mono">{{ t.label }}</span>
    </div>

    <div
      v-if="timeline.diff"
      class="mt-2 max-h-24 overflow-y-auto rounded-xs border border-violet/30 bg-elevated/60 p-2 text-[11px]"
    >
      <p class="text-violet">
        差异 {{ timeline.diff.summary.total_difference }}：
        +{{ timeline.diff.summary.nodes_added }} 资产
        / -{{ timeline.diff.summary.nodes_removed }} 资产
        / +{{ timeline.diff.summary.edges_added }} 关系
        / -{{ timeline.diff.summary.edges_removed }} 关系
      </p>
      <p
        v-for="n in timeline.diff.added_nodes.slice(0, 4)"
        :key="'a' + n.id"
        class="text-signal"
      >
        + {{ n.name }}
      </p>
      <p
        v-for="n in timeline.diff.removed_nodes.slice(0, 4)"
        :key="'r' + n.id"
        class="text-danger"
      >
        − {{ n.name }}
      </p>
    </div>
  </footer>
</template>
