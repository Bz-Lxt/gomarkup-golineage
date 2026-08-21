<script setup lang="ts">
import { computed } from 'vue'

import { formatDateTime, formatNumber } from '@/utils/format'
import { useGraphStore } from '@/stores/graph'
import { useTimelineStore } from '@/stores/timeline'

const graph = useGraphStore()
const timeline = useTimelineStore()

const connected = computed(() => !!graph.metadata && !graph.initError)
const nodeCount = computed(() => graph.stats?.node_count ?? 0)
const edgeCount = computed(() => graph.stats?.edge_count ?? 0)
const eventCount = computed(() => timeline.bounds?.event_count ?? 0)
const serverTime = computed(() => formatDateTime(graph.metadata?.server_time))
</script>

<template>
  <header class="scanline panel flex h-12 shrink-0 items-center gap-4 border-b px-4">
    <div class="flex items-center gap-2.5">
      <svg viewBox="0 0 24 24" class="size-5" fill="none" aria-hidden="true">
        <path d="M6 6 L18 12 L6 18 Z" stroke="var(--color-signal)" stroke-width="1.4" />
        <circle cx="6" cy="6" r="2.2" fill="#5B8FF9" />
        <circle cx="18" cy="12" r="2.2" fill="#F6BD16" />
        <circle cx="6" cy="18" r="2.2" fill="#5AD8A6" />
      </svg>
      <div>
        <h1 class="text-[14px] leading-none tracking-[0.18em]">GOLINEAGE</h1>
        <p class="label-caps mt-0.5 !normal-case !tracking-[0.08em]">资产与全路径血缘分析</p>
      </div>
    </div>

    <div class="mx-2 hidden h-6 w-px bg-line md:block" />

    <div class="hidden items-center gap-4 md:flex">
      <div>
        <p class="label-caps">资产</p>
        <p class="mono text-[13px] text-ink">{{ formatNumber(nodeCount) }}</p>
      </div>
      <div>
        <p class="label-caps">关系</p>
        <p class="mono text-[13px] text-ink">{{ formatNumber(edgeCount) }}</p>
      </div>
      <div>
        <p class="label-caps">流水</p>
        <p class="mono text-[13px] text-ink">{{ formatNumber(eventCount) }}</p>
      </div>
    </div>

    <div class="ml-auto flex items-center gap-3">
      <p class="mono hidden text-[11px] text-ink-mute lg:block">{{ serverTime }}</p>
      <div
        class="flex items-center gap-1.5 rounded-xs border px-2 py-1"
        :class="connected ? 'border-signal/30' : 'border-danger/40'"
      >
        <span
          class="size-1.5 rounded-full"
          :class="connected ? 'bg-signal anim-breathe' : 'bg-danger'"
        />
        <span class="label-caps !normal-case" :class="connected ? '!text-signal' : '!text-danger'">
          {{ connected ? 'LIVE' : graph.initError || 'OFFLINE' }}
        </span>
      </div>
    </div>
  </header>
</template>
