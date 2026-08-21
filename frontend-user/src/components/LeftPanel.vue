<script setup lang="ts">
import { computed, ref } from 'vue'

import AppButton from '@/components/base/AppButton.vue'
import AppInput from '@/components/base/AppInput.vue'
import AppSelect from '@/components/base/AppSelect.vue'
import AppTag from '@/components/base/AppTag.vue'
import { useGraphStore } from '@/stores/graph'
import { useTimelineStore } from '@/stores/timeline'
import { NODE_LABELS, nodeColor, nodeLabel } from '@/utils/palette'

const emit = defineEmits<{ createNode: []; createEdge: [] }>()

const graph = useGraphStore()
const timeline = useTimelineStore()

const keyword = ref('')
const collapsed = ref(false)

const typeOptions = computed(() =>
  (graph.metadata?.node_types ?? Object.keys(NODE_LABELS)).map((t) => ({
    value: t,
    label: `${nodeLabel(t)}  ${graph.stats?.type_counts?.[t] ?? 0}`,
    color: nodeColor(t),
    count: graph.stats?.type_counts?.[t] ?? 0,
  })),
)

const filtered = computed(() => {
  const q = keyword.value.trim().toLowerCase()
  if (!q) return graph.nodes.slice(0, 24)
  return graph.nodes
    .filter(
      (n) =>
        n.name.toLowerCase().includes(q) ||
        n.id.toLowerCase().includes(q) ||
        Object.values(n.properties ?? {}).some((v) => String(v).toLowerCase().includes(q)),
    )
    .slice(0, 24)
})

const relationOptions = computed(() =>
  (graph.metadata?.relation_types ?? []).map((r) => ({ value: r, label: r })),
)

function pick(id: string) {
  graph.selectNode(id)
}
</script>

<template>
  <aside
    class="panel relative z-20 flex h-full shrink-0 flex-col border-r max-md:absolute max-md:inset-y-0 max-md:left-0"
    :class="collapsed ? 'w-10' : 'w-60'"
  >
    <button
      type="button"
      class="absolute top-2 -right-3 z-10 grid size-6 place-items-center rounded-xs border border-line bg-elevated text-ink-mute hover:text-ink"
      :aria-label="collapsed ? '展开侧栏' : '收起侧栏'"
      @click="collapsed = !collapsed"
    >
      <span class="text-[10px]">{{ collapsed ? '›' : '‹' }}</span>
    </button>

    <template v-if="!collapsed">
      <section class="border-b border-line p-3">
        <p class="label-caps mb-2">检索资产</p>
        <AppInput v-model="keyword" placeholder="名称 / ID / 属性" mono />
        <p class="mt-1.5 text-[10px] text-ink-mute">本地过滤当前拓扑，不发起查询</p>
      </section>

      <section class="min-h-0 flex-1 overflow-y-auto border-b border-line">
        <button
          v-for="n in filtered"
          :key="n.id"
          type="button"
          class="flex w-full items-center gap-2 px-3 py-1.5 text-left transition-colors hover:bg-elevated"
          :class="graph.selectedNodeId === n.id && 'bg-signal/10'"
          @click="pick(n.id)"
        >
          <span class="size-2 shrink-0 rounded-full" :style="{ backgroundColor: nodeColor(n.type) }" />
          <span class="min-w-0 flex-1 truncate text-[12px]">{{ n.name }}</span>
          <span class="label-caps !normal-case">{{ nodeLabel(n.type) }}</span>
        </button>
        <p v-if="filtered.length === 0" class="px-3 py-4 text-center text-[11px] text-ink-mute">
          无匹配资产
        </p>
      </section>

      <section class="border-b border-line p-3">
        <div class="mb-2 flex items-center justify-between">
          <p class="label-caps">类型图例</p>
          <button
            v-if="graph.typeFilter.size"
            type="button"
            class="text-[10px] text-signal hover:underline"
            @click="graph.resetTypeFilter()"
          >
            清除
          </button>
        </div>
        <div class="flex flex-wrap gap-1">
          <button
            v-for="opt in typeOptions"
            :key="opt.value"
            type="button"
            @click="graph.toggleTypeFilter(opt.value)"
          >
            <AppTag
              :color="opt.color"
              dot
              :variant="graph.typeFilter.size === 0 || graph.typeFilter.has(opt.value) ? 'solid' : 'outline'"
            >
              {{ nodeLabel(opt.value) }} {{ opt.count }}
            </AppTag>
          </button>
        </div>
        <p class="mt-2 text-[10px] text-ink-mute">点击图例过滤画布，可多选</p>
      </section>

      <section class="border-b border-line p-3">
        <p class="label-caps mb-2">最短 / 全路径</p>
        <AppSelect
          v-model="graph.pathFrom"
          :options="graph.nodeOptions"
          placeholder="起点"
        />
        <div class="mt-1.5">
          <AppSelect
            v-model="graph.pathTo"
            :options="graph.nodeOptions"
            placeholder="终点"
          />
        </div>
        <div class="mt-2 grid grid-cols-2 gap-1.5">
          <AppButton variant="primary" :loading="graph.pathLoading" @click="graph.findShortestPath()">
            最短路径
          </AppButton>
          <AppButton :loading="graph.pathLoading" @click="graph.findAllPaths()">全路径</AppButton>
        </div>
        <p v-if="graph.pathResult?.found" class="mono mt-2 text-[11px] text-amber">
          {{ graph.pathResult.hops }} 跳 · 代价 {{ graph.pathResult.total_cost }}
        </p>
        <p v-else-if="graph.allPathsResult" class="mono mt-2 text-[11px] text-amber">
          {{ graph.allPathsResult.paths.length }} 条路径
        </p>
      </section>

      <section class="p-3">
        <p class="label-caps mb-2">变更</p>
        <div class="grid grid-cols-2 gap-1.5">
          <AppButton
            variant="primary"
            :disabled="timeline.historyMode"
            title="历史视图下禁止写入"
            @click="emit('createNode')"
          >
            新建资产
          </AppButton>
          <AppButton :disabled="timeline.historyMode" @click="emit('createEdge')">建立关系</AppButton>
        </div>
        <p v-if="timeline.historyMode" class="mt-2 text-[10px] text-violet">
          当前为历史快照，写入已锁定
        </p>
        <p v-else class="mt-2 hidden text-[10px] text-ink-mute">
          {{ relationOptions.length }} 种关系类型可用
        </p>
      </section>
    </template>
  </aside>
</template>
