<script setup lang="ts">
import { computed, ref, watch } from 'vue'

import AppInput from '@/components/base/AppInput.vue'
import AppModal from '@/components/base/AppModal.vue'
import AppSelect from '@/components/base/AppSelect.vue'
import { useToast } from '@/composables/useToast'
import { useGraphStore } from '@/stores/graph'
import { useTimelineStore } from '@/stores/timeline'
import { NODE_LABELS, nodeLabel } from '@/utils/palette'
import {
  hasErrors,
  rowsToProperties,
  summarize,
  validateName,
  validateProperties,
  type PropertyRow,
} from '@/utils/validators'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: [] }>()

const graph = useGraphStore()
const timeline = useTimelineStore()
const toast = useToast()

const name = ref('')
const type = ref('application')
const nameError = ref('')
const typeError = ref('')
const rows = ref<PropertyRow[]>([
  { key: 'owner', value: '', type: 'string' },
  { key: 'risk_level', value: 'low', type: 'string' },
])
const rowErrors = ref<Record<string, string>>({})
const loading = ref(false)

const typeOptions = computed(() =>
  (graph.metadata?.node_types ?? Object.keys(NODE_LABELS)).map((t) => ({
    value: t,
    label: nodeLabel(t),
  })),
)

watch(
  () => props.open,
  (open) => {
    if (!open) return
    name.value = ''
    type.value = graph.metadata?.node_types?.[2] ?? 'application'
    nameError.value = ''
    typeError.value = ''
    rowErrors.value = {}
    rows.value = [
      { key: 'owner', value: '', type: 'string' },
      { key: 'risk_level', value: 'low', type: 'string' },
    ]
  },
)

async function confirm() {
  nameError.value = validateName(name.value) ?? ''
  typeError.value = type.value ? '' : '请选择资产类型'
  rowErrors.value = validateProperties(rows.value.filter((r) => r.key.trim() || r.value.trim()))
  if (nameError.value || typeError.value || hasErrors(rowErrors.value)) {
    toast.error(nameError.value || typeError.value || summarize(rowErrors.value))
    return
  }
  loading.value = true
  try {
    await graph.createNode({
      name: name.value.trim(),
      type: type.value as never,
      properties: rowsToProperties(rows.value.filter((r) => r.key.trim())),
      reason: '工作台新建资产',
    })
    await timeline.loadEvents()
    await timeline.loadBounds()
    emit('close')
  } catch (err) {
    graph.reportError(err, '新建资产失败')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <AppModal
    :open="open"
    title="新建资产"
    message="资产会立即写入事件日志并出现在当前拓扑中。"
    confirm-text="创建"
    :loading="loading"
    @cancel="emit('close')"
    @confirm="confirm"
  >
    <div class="mt-3 space-y-2">
      <AppInput v-model="name" label="名称 *" :error="nameError" placeholder="例如：结算服务" />
      <AppSelect v-model="type" label="类型 *" :options="typeOptions" :error="typeError" />
      <AppInput
        :model-value="rows[0].value"
        label="责任人"
        placeholder="例如：运维-王强"
        @update:model-value="rows[0].value = $event"
      />
      <AppSelect
        :model-value="rows[1].value"
        label="风险等级"
        :options="[
          { value: 'low', label: '低' },
          { value: 'medium', label: '中' },
          { value: 'high', label: '高' },
          { value: 'critical', label: '严重' },
        ]"
        @update:model-value="rows[1].value = $event"
      />
    </div>
  </AppModal>
</template>
