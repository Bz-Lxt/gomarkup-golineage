<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'

import AppSpinner from '@/components/base/AppSpinner.vue'
import { LAYOUT_OPTIONS, type LayoutName, useGraphCanvas } from '@/composables/useGraphCanvas'
import { useGraphStore } from '@/stores/graph'
import { useTimelineStore } from '@/stores/timeline'

const graphStore = useGraphStore()
const timeline = useTimelineStore()
const canvas = useGraphCanvas()

const container = ref<HTMLElement | null>(null)
const layout = ref<LayoutName>('force')
const zoomLabel = ref('100%')
const rendering = ref(false)

/** 时间轴切换时掠过的扫描线，给「画布被重建」一个视觉交代。 */
const sweeping = ref(false)

function syncZoom() {
  const g = canvas.instance.value
  if (!g) return
  zoomLabel.value = `${Math.round(g.getZoom() * 100)}%`
}

async function draw(relayout: boolean) {
  const g = canvas.instance.value
  if (!g) return
  rendering.value = true
  try {
    g.setData(canvas.buildData(graphStore.nodes, graphStore.edges))
    if (relayout) await g.render()
    else await g.draw()
    await g.fitView()
    applyStates()
    syncZoom()
  } finally {
    rendering.value = false
  }
}

function applyStates() {
  canvas.applyStates(graphStore.nodes, graphStore.edges, {
    focusNodes: graphStore.focusNodes,
    focusEdges: graphStore.focusEdges,
    selectedNodeId: graphStore.selectedNodeId,
    selectedEdgeId: graphStore.selectedEdgeId,
    pathMode: graphStore.focusMode === 'path',
    hiddenNodes: graphStore.visibleNodeIds,
  })
}

onMounted(async () => {
  if (!container.value) return
  const g = canvas.create(container.value, layout.value, {
    onNodeClick: (id) => graphStore.selectNode(id),
    onEdgeClick: (id) => graphStore.selectEdge(id),
    onCanvasClick: () => {
      graphStore.selectNode(null)
      graphStore.clearFocus()
    },
    onNodeDoubleClick: (id) => graphStore.analyzeLineage(id),
  })
  g.on('afterrender', syncZoom)
  await draw(true)
})

onBeforeUnmount(() => canvas.destroy())

// 拓扑数据整体替换（初次加载、刷新、时间轴回溯）时需要重新布局。
watch(
  () => graphStore.topology,
  async () => {
    sweeping.value = true
    await draw(true)
    setTimeout(() => (sweeping.value = false), 600)
  },
)

// 高亮变化只改状态，绝不重新布局，否则节点位置会跳。
watch(
  [
    () => graphStore.focusNodes,
    () => graphStore.focusEdges,
    () => graphStore.selectedNodeId,
    () => graphStore.selectedEdgeId,
    () => graphStore.visibleNodeIds,
  ],
  applyStates,
)

watch(layout, async (name) => {
  const g = canvas.instance.value
  if (!g) return
  rendering.value = true
  try {
    g.setLayout(canvas.layoutConfig(name) as never)
    await g.layout()
    applyStates()
  } finally {
    rendering.value = false
  }
})

function zoomBy(ratio: number) {
  const g = canvas.instance.value
  if (!g) return
  g.zoomBy(ratio)
  syncZoom()
}

async function fitView() {
  const g = canvas.instance.value
  if (!g) return
  await g.fitView()
  syncZoom()
}
</script>

<template>
  <div class="relative min-w-0 flex-1">
    <div ref="container" class="grid-field absolute inset-0" />
    <div class="vignette pointer-events-none absolute inset-0" />

    <!-- 时间轴回溯时的扫描线 -->
    <div
      v-if="sweeping"
      class="pointer-events-none absolute inset-0 overflow-hidden"
      aria-hidden="true"
    >
      <div
        class="h-full w-1/3 bg-gradient-to-r from-transparent via-violet/10 to-transparent"
        style="animation: sweep 600ms ease-out"
      />
    </div>

    <!-- 历史模式角标：颜色语义与时间轴一致，防止误认为当前拓扑 -->
    <div
      v-if="timeline.historyMode"
      class="pointer-events-none absolute top-3 left-3 flex items-center gap-2 rounded-xs border border-violet/40 bg-void/85 px-2.5 py-1.5"
    >
      <span class="anim-breathe size-1.5 rounded-full bg-violet" />
      <span class="label-caps !text-violet">历史视图 · 只读</span>
    </div>

    <!-- 视图工具条 -->
    <div class="absolute top-3 right-3 flex items-center gap-1.5">
      <div class="panel corner-cut flex items-center rounded-xs">
        <button
          v-for="opt in LAYOUT_OPTIONS"
          :key="opt.value"
          type="button"
          class="h-7 px-2.5 text-[11px] transition-colors first:rounded-l-xs last:rounded-r-xs"
          :class="
            layout === opt.value
              ? 'bg-signal/15 text-signal'
              : 'text-ink-mute hover:bg-elevated hover:text-ink'
          "
          @click="layout = opt.value"
        >
          {{ opt.label }}
        </button>
      </div>

      <div class="panel corner-cut flex items-center rounded-xs">
        <button
          type="button"
          aria-label="缩小"
          class="grid size-7 place-items-center text-ink-mute transition-colors hover:bg-elevated hover:text-ink"
          @click="zoomBy(0.8)"
        >
          <svg viewBox="0 0 12 12" class="size-3" stroke="currentColor" stroke-width="1.5">
            <path d="M2 6h8" stroke-linecap="round" />
          </svg>
        </button>
        <span class="mono w-12 text-center text-[10px] text-ink-dim">{{ zoomLabel }}</span>
        <button
          type="button"
          aria-label="放大"
          class="grid size-7 place-items-center text-ink-mute transition-colors hover:bg-elevated hover:text-ink"
          @click="zoomBy(1.25)"
        >
          <svg viewBox="0 0 12 12" class="size-3" stroke="currentColor" stroke-width="1.5">
            <path d="M6 2v8M2 6h8" stroke-linecap="round" />
          </svg>
        </button>
        <button
          type="button"
          title="适应画布"
          aria-label="适应画布"
          class="grid size-7 place-items-center border-l border-line text-ink-mute transition-colors hover:bg-elevated hover:text-ink"
          @click="fitView"
        >
          <svg viewBox="0 0 12 12" class="size-3" fill="none" stroke="currentColor" stroke-width="1.5">
            <path d="M1.5 4V1.5H4M8 1.5h2.5V4M10.5 8v2.5H8M4 10.5H1.5V8" stroke-linecap="round" />
          </svg>
        </button>
      </div>
    </div>

    <!-- 空态：明确告诉用户下一步做什么，而不是留一片黑 -->
    <div
      v-if="!graphStore.loading && graphStore.nodes.length === 0"
      class="pointer-events-none absolute inset-0 grid place-items-center"
    >
      <div class="text-center">
        <p class="label-caps mb-1">画布为空</p>
        <p class="text-[12px] text-ink-mute">
          {{
            timeline.historyMode
              ? '该时刻尚无任何资产，试着把时间轴往右拖'
              : '尚无资产数据，可在左侧新建资产'
          }}
        </p>
      </div>
    </div>

    <div
      v-if="graphStore.loading || rendering || timeline.snapshotLoading"
      class="pointer-events-none absolute bottom-3 left-3"
    >
      <div class="panel corner-cut flex items-center gap-2 rounded-xs px-2.5 py-1.5">
        <AppSpinner :size="12" />
        <span class="text-[11px] text-ink-dim">
          {{ timeline.snapshotLoading ? '重放事件中' : '渲染中' }}
        </span>
      </div>
    </div>

    <!-- 操作提示：双击是隐藏能力，必须显式告知 -->
    <div class="pointer-events-none absolute right-3 bottom-3 text-right">
      <p class="text-[10px] text-ink-mute">单击高亮邻居 · 双击分析血缘 · 拖拽移动节点</p>
    </div>
  </div>
</template>
