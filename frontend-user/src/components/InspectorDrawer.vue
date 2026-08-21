<script setup lang="ts">
import { computed, ref, watch } from 'vue'

import AppButton from '@/components/base/AppButton.vue'
import AppDrawer from '@/components/base/AppDrawer.vue'
import AppInput from '@/components/base/AppInput.vue'
import AppModal from '@/components/base/AppModal.vue'
import AppSelect from '@/components/base/AppSelect.vue'
import AppSpinner from '@/components/base/AppSpinner.vue'
import AppTag from '@/components/base/AppTag.vue'
import { useToast } from '@/composables/useToast'
import { api } from '@/api/client'
import type { LineageEvent } from '@/api/types'
import { useGraphStore } from '@/stores/graph'
import { useTimelineStore } from '@/stores/timeline'
import { formatDateTime, formatPropertyValue } from '@/utils/format'
import { eventColor, eventLabel, nodeColor, nodeLabel, relationLabel, riskOf, RISK_COLORS, RISK_LABELS } from '@/utils/palette'
import {
  hasErrors,
  rowsToProperties,
  summarize,
  toPropertyRows,
  validateName,
  validateProperties,
  type PropertyRow,
} from '@/utils/validators'

const graph = useGraphStore()
const timeline = useTimelineStore()
const toast = useToast()

const tab = ref<'props' | 'lineage' | 'events'>('props')
const name = ref('')
const nameError = ref('')
const rows = ref<PropertyRow[]>([])
const rowErrors = ref<Record<string, string>>({})
const saving = ref(false)
const events = ref<LineageEvent[]>([])
const eventsLoading = ref(false)

const confirmOpen = ref(false)
const confirmKind = ref<'node' | 'edge'>('node')
const confirmReason = ref('')
const deleting = ref(false)

const open = computed(() => !!graph.selectedNode || !!graph.selectedEdge)
const title = computed(() => {
  if (graph.selectedNode) return graph.selectedNode.name
  if (graph.selectedEdge) return relationLabel(graph.selectedEdge.relation)
  return '详情'
})
const subtitle = computed(() => graph.selectedNode?.id ?? graph.selectedEdge?.id ?? '')
const readonly = computed(() => timeline.historyMode)
const risk = computed(() => riskOf(graph.selectedNode?.properties))

const typeOptions = computed(() =>
  ['string', 'number', 'boolean'].map((v) => ({
    value: v,
    label: v === 'string' ? '文本' : v === 'number' ? '数字' : '布尔',
  })),
)

watch(
  () => graph.selectedNode,
  (node) => {
    if (!node) return
    tab.value = 'props'
    name.value = node.name
    nameError.value = ''
    rows.value = toPropertyRows(node.properties)
    rowErrors.value = {}
    events.value = []
  },
)

watch(
  () => graph.selectedEdge,
  (edge) => {
    if (!edge) return
    tab.value = 'props'
    events.value = []
  },
)

function addRow() {
  rows.value = [...rows.value, { key: '', value: '', type: 'string' }]
}

function removeRow(index: number) {
  rows.value = rows.value.filter((_, i) => i !== index)
}

async function save() {
  if (!graph.selectedNode || readonly.value) return
  nameError.value = validateName(name.value) ?? ''
  rowErrors.value = validateProperties(rows.value)
  if (nameError.value || hasErrors(rowErrors.value)) {
    toast.error(nameError.value || summarize(rowErrors.value))
    return
  }
  saving.value = true
  try {
    await graph.updateNode(graph.selectedNode.id, {
      name: name.value.trim(),
      properties: rowsToProperties(rows.value),
      replace_properties: true,
      reason: '抽屉属性编辑',
    })
    await timeline.loadEvents()
    await timeline.loadBounds()
  } catch (err) {
    graph.reportError(err, '保存属性失败')
  } finally {
    saving.value = false
  }
}

async function loadEvents() {
  const id = graph.selectedNode?.id ?? graph.selectedEdge?.id
  if (!id) return
  eventsLoading.value = true
  try {
    const page = graph.selectedNode
      ? await api.nodeEvents(id, 40)
      : await api.events({ entity_id: id, limit: 40 })
    events.value = page.items
  } catch (err) {
    graph.reportError(err, '读取变更流水失败')
  } finally {
    eventsLoading.value = false
  }
}

watch(tab, (t) => {
  if (t === 'events' && events.value.length === 0) void loadEvents()
  if (t === 'lineage' && graph.selectedNode && !graph.lineageResult) {
    void graph.analyzeLineage(graph.selectedNode.id)
  }
})

function askDelete(kind: 'node' | 'edge') {
  confirmKind.value = kind
  confirmReason.value = kind === 'edge' ? '资产关系发生变化，解除调用' : ''
  confirmOpen.value = true
}

async function confirmDelete() {
  deleting.value = true
  try {
    if (confirmKind.value === 'node' && graph.selectedNode) {
      await graph.removeNode(graph.selectedNode.id, confirmReason.value)
    } else if (graph.selectedEdge) {
      await graph.removeEdge(graph.selectedEdge.id, confirmReason.value)
    }
    confirmOpen.value = false
    await timeline.loadEvents()
    await timeline.loadBounds()
  } catch (err) {
    graph.reportError(err, '删除失败')
  } finally {
    deleting.value = false
  }
}

function close() {
  graph.selectNode(null)
  graph.selectEdge(null)
  graph.clearFocus()
}
</script>

<template>
  <AppDrawer :open="open" :title="title" :subtitle="subtitle" :width="360" @close="close">
    <template v-if="graph.selectedNode">
      <div class="flex items-center gap-2 border-b border-line px-3 py-2">
        <AppTag :color="nodeColor(graph.selectedNode.type)" dot>
          {{ nodeLabel(graph.selectedNode.type) }}
        </AppTag>
        <AppTag v-if="risk" :color="RISK_COLORS[risk]" variant="solid">
          风险 {{ RISK_LABELS[risk] }}
        </AppTag>
        <span class="ml-auto mono text-[10px] text-ink-mute">
          {{ graph.nodeDetail?.in_degree ?? 0 }} 入 / {{ graph.nodeDetail?.out_degree ?? 0 }} 出
        </span>
      </div>

      <nav class="flex border-b border-line">
        <button
          v-for="item in [
            { id: 'props', label: '属性' },
            { id: 'lineage', label: '血缘' },
            { id: 'events', label: '流水' },
          ] as const"
          :key="item.id"
          type="button"
          class="h-8 flex-1 text-[11px] tracking-wide"
          :class="tab === item.id ? 'border-b-2 border-signal text-signal' : 'text-ink-mute hover:text-ink'"
          @click="tab = item.id"
        >
          {{ item.label }}
        </button>
      </nav>

      <div v-if="graph.detailLoading" class="grid place-items-center py-10">
        <AppSpinner label="读取详情" />
      </div>

      <div v-else-if="tab === 'props'" class="space-y-3 p-3">
        <AppInput v-model="name" label="资产名称" :error="nameError" :disabled="readonly" />

        <div class="flex items-center justify-between">
          <p class="label-caps">动态属性</p>
          <AppButton size="sm" :disabled="readonly" @click="addRow">添加属性</AppButton>
        </div>
        <p class="text-[10px] text-ink-mute">
          键名仅允许字母、数字、下划线、连字符与中文。可添加 IP、责任人、风险等级等。
        </p>

        <div v-for="(row, i) in rows" :key="i" class="grid grid-cols-[1fr_72px_1fr_24px] items-start gap-1.5">
          <AppInput
            :model-value="row.key"
            placeholder="键"
            mono
            :disabled="readonly"
            :error="rowErrors[`key-${i}`]"
            @update:model-value="rows[i].key = $event"
          />
          <AppSelect
            :model-value="row.type"
            :options="typeOptions"
            :disabled="readonly"
            @update:model-value="rows[i].type = $event as PropertyRow['type']"
          />
          <AppInput
            :model-value="row.value"
            placeholder="值"
            :disabled="readonly"
            :error="rowErrors[`value-${i}`]"
            @update:model-value="rows[i].value = $event"
          />
          <button
            type="button"
            class="mt-0.5 grid size-6 place-items-center text-ink-mute hover:text-danger"
            :disabled="readonly"
            aria-label="删除属性"
            @click="removeRow(i)"
          >
            ×
          </button>
        </div>
        <p v-if="rows.length === 0" class="py-4 text-center text-[11px] text-ink-mute">尚无自定义属性</p>
      </div>

      <div v-else-if="tab === 'lineage'" class="space-y-3 p-3">
        <div v-if="graph.lineageLoading" class="grid place-items-center py-8">
          <AppSpinner label="分析血缘" />
        </div>
        <template v-else-if="graph.lineageResult">
          <p class="text-[12px] text-ink-dim">
            上游 {{ graph.lineageResult.upstream_count }} · 下游 {{ graph.lineageResult.downstream_count }}
          </p>
          <div>
            <p class="label-caps mb-1">上游（影响源）</p>
            <p
              v-for="n in graph.lineageResult.upstream"
              :key="n.id"
              class="flex items-center gap-2 py-0.5 text-[12px]"
            >
              <span class="size-1.5 rounded-full" :style="{ backgroundColor: nodeColor(n.type) }" />
              {{ n.name }}
              <span class="mono text-[10px] text-ink-mute">L{{ graph.lineageResult.levels[n.id] }}</span>
            </p>
            <p v-if="!graph.lineageResult.upstream.length" class="text-[11px] text-ink-mute">无上游</p>
          </div>
          <div>
            <p class="label-caps mb-1">下游（影响面）</p>
            <p
              v-for="n in graph.lineageResult.downstream"
              :key="n.id"
              class="flex items-center gap-2 py-0.5 text-[12px]"
            >
              <span class="size-1.5 rounded-full" :style="{ backgroundColor: nodeColor(n.type) }" />
              {{ n.name }}
              <span class="mono text-[10px] text-ink-mute">L{{ graph.lineageResult.levels[n.id] }}</span>
            </p>
            <p v-if="!graph.lineageResult.downstream.length" class="text-[11px] text-ink-mute">无下游</p>
          </div>
        </template>
      </div>

      <div v-else class="space-y-2 p-3">
        <div v-if="eventsLoading" class="grid place-items-center py-8">
          <AppSpinner label="读取流水" />
        </div>
        <article v-for="ev in events" :key="ev.seq" class="border-l-2 pl-2" :style="{ borderColor: eventColor(ev.event_type) }">
          <p class="text-[12px]" :style="{ color: eventColor(ev.event_type) }">
            {{ ev.event_label || eventLabel(ev.event_type) }}
          </p>
          <p class="mono text-[10px] text-ink-mute">
            #{{ ev.seq }} · {{ ev.actor }} · {{ formatDateTime(ev.occurred_at) }}
          </p>
          <p v-if="ev.reason" class="text-[11px] text-ink-dim">{{ ev.reason }}</p>
        </article>
        <p v-if="!eventsLoading && events.length === 0" class="py-6 text-center text-[11px] text-ink-mute">
          暂无流水
        </p>
      </div>
    </template>

    <template v-else-if="graph.selectedEdge">
      <div class="space-y-3 p-3">
        <div class="flex items-center gap-2">
          <AppTag>{{ relationLabel(graph.selectedEdge.relation) }}</AppTag>
          <span class="mono text-[10px] text-ink-mute">权重 {{ graph.selectedEdge.weight }}</span>
        </div>
        <p class="text-[12px] text-ink-dim">
          {{ graph.nodeMap.get(graph.selectedEdge.source_id)?.name ?? graph.selectedEdge.source_id }}
          →
          {{ graph.nodeMap.get(graph.selectedEdge.target_id)?.name ?? graph.selectedEdge.target_id }}
        </p>
        <div v-if="graph.selectedEdge.properties">
          <p class="label-caps mb-1">属性</p>
          <p
            v-for="(v, k) in graph.selectedEdge.properties"
            :key="k"
            class="flex justify-between text-[12px]"
          >
            <span class="mono text-ink-mute">{{ k }}</span>
            <span>{{ formatPropertyValue(v) }}</span>
          </p>
        </div>
      </div>
    </template>

    <template #footer>
      <div v-if="graph.selectedNode" class="flex gap-2">
        <AppButton variant="primary" block :loading="saving" :disabled="readonly" @click="save">
          保存属性
        </AppButton>
        <AppButton variant="danger" :disabled="readonly" @click="askDelete('node')">删除</AppButton>
      </div>
      <div v-else class="flex gap-2">
        <AppButton variant="danger" block :disabled="readonly" @click="askDelete('edge')">解除关系</AppButton>
      </div>
    </template>
  </AppDrawer>

  <AppModal
    :open="confirmOpen"
    :title="confirmKind === 'node' ? '删除资产' : '解除关系'"
    :message="
      confirmKind === 'node'
        ? '删除后将级联解除该资产上的全部关系，操作会写入变更流水，可按时间轴回溯。'
        : '解除后当前拓扑不再包含这条边，历史快照仍可还原。'
    "
    danger
    :loading="deleting"
    confirm-text="确认执行"
    @cancel="confirmOpen = false"
    @confirm="confirmDelete"
  >
    <div class="mt-3">
      <AppInput v-model="confirmReason" label="变更原因（写入流水）" placeholder="例如：A 应用不再调用 B 数据库" />
    </div>
  </AppModal>
</template>
