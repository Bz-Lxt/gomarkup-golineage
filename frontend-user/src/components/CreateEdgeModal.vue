<script setup lang="ts">
import { computed, ref, watch } from 'vue'

import AppInput from '@/components/base/AppInput.vue'
import AppModal from '@/components/base/AppModal.vue'
import AppSelect from '@/components/base/AppSelect.vue'
import { useToast } from '@/composables/useToast'
import { useGraphStore } from '@/stores/graph'
import { useTimelineStore } from '@/stores/timeline'
import { relationLabel } from '@/utils/palette'
import { validateWeight } from '@/utils/validators'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: [] }>()

const graph = useGraphStore()
const timeline = useTimelineStore()
const toast = useToast()

const source = ref('')
const target = ref('')
const relation = ref('calls')
const weight = ref('1')
const reason = ref('')
const errors = ref<Record<string, string>>({})
const loading = ref(false)

const relationOptions = computed(() =>
  (graph.metadata?.relation_types ?? []).map((r) => ({ value: r, label: relationLabel(r) })),
)

watch(
  () => props.open,
  (open) => {
    if (!open) return
    source.value = graph.selectedNodeId ?? ''
    target.value = ''
    relation.value = graph.metadata?.relation_types?.[1] ?? 'calls'
    weight.value = '1'
    reason.value = ''
    errors.value = {}
  },
)

async function confirm() {
  const next: Record<string, string> = {}
  if (!source.value) next.source = '请选择起点'
  if (!target.value) next.target = '请选择终点'
  if (source.value && source.value === target.value) next.target = '起点与终点不能相同'
  if (!relation.value) next.relation = '请选择关系类型'
  const w = validateWeight(weight.value)
  if (w) next.weight = w
  errors.value = next
  if (Object.keys(next).length) {
    toast.error(Object.values(next)[0])
    return
  }
  loading.value = true
  try {
    await graph.createEdge({
      source_id: source.value,
      target_id: target.value,
      relation: relation.value as never,
      weight: Number(weight.value),
      reason: reason.value || '工作台建立关系',
    })
    await timeline.loadEvents()
    await timeline.loadBounds()
    emit('close')
  } catch (err) {
    graph.reportError(err, '建立关系失败')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <AppModal
    :open="open"
    title="建立血缘关系"
    message="关系会写入不可变事件日志，解除后仍可按时间轴回溯。"
    confirm-text="建立"
    :loading="loading"
    @cancel="emit('close')"
    @confirm="confirm"
  >
    <div class="mt-3 space-y-2">
      <AppSelect v-model="source" label="起点 *" :options="graph.nodeOptions" :error="errors.source" placeholder="选择资产" />
      <AppSelect v-model="target" label="终点 *" :options="graph.nodeOptions" :error="errors.target" placeholder="选择资产" />
      <AppSelect v-model="relation" label="关系 *" :options="relationOptions" :error="errors.relation" />
      <AppInput v-model="weight" label="权重 *" :error="errors.weight" mono />
      <AppInput v-model="reason" label="变更原因" placeholder="例如：订单服务开始调用结算库" />
    </div>
  </AppModal>
</template>
