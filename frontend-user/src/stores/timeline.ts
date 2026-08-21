/**
 * 时间轴回溯状态。
 *
 * 核心不变式：`historyMode` 为 true 时，画布展示的是历史快照，
 * 一切写操作必须被禁用 —— 在历史视图上编辑是没有意义的，
 * 而且会让用户误以为自己在「修改过去」。
 */
import { defineStore } from 'pinia'
import { computed, ref, shallowRef } from 'vue'

import { ApiError, api } from '@/api/client'
import type { EventPage, LineageEvent, TimelineBounds, TopologyDiff } from '@/api/types'
import { useToast } from '@/composables/useToast'
import { toRFC3339Beijing } from '@/utils/format'

import { useGraphStore } from './graph'

export const useTimelineStore = defineStore('timeline', () => {
  const toast = useToast()
  const graph = useGraphStore()

  const bounds = ref<TimelineBounds | null>(null)
  const events = shallowRef<LineageEvent[]>([])
  const eventTotal = ref(0)
  const eventsLoading = ref(false)

  /** 滑块当前时刻（毫秒时间戳）。 */
  const cursor = ref(Date.now())
  const historyMode = ref(false)
  const snapshotLoading = ref(false)
  const snapshotMeta = ref<{ events_applied: number; duration_ms: number } | null>(null)

  const diff = ref<TopologyDiff | null>(null)
  const diffLoading = ref(false)

  const minTime = computed(() =>
    bounds.value?.earliest ? new Date(bounds.value.earliest).getTime() : Date.now() - 86400000,
  )
  const maxTime = computed(() => {
    // 上界取「最后一条事件」与「此刻」的较大值：
    // 否则用户刚做的修改会落在滑块右侧之外，拖到最右也回不到当前态。
    const latest = bounds.value?.latest ? new Date(bounds.value.latest).getTime() : 0
    return Math.max(latest, Date.now())
  })

  const cursorISO = computed(() => toRFC3339Beijing(cursor.value))

  function reportError(err: unknown, fallback: string) {
    if (err instanceof ApiError) toast.error(err.message || fallback, err.traceId)
    else toast.error(fallback)
  }

  async function loadBounds() {
    try {
      bounds.value = await api.bounds()
      cursor.value = maxTime.value
    } catch (err) {
      reportError(err, '读取时间轴范围失败')
    }
  }

  async function loadEvents(limit = 80) {
    eventsLoading.value = true
    try {
      const page: EventPage = await api.events({ limit })
      events.value = page.items
      eventTotal.value = page.total
    } catch (err) {
      reportError(err, '读取变更流水失败')
    } finally {
      eventsLoading.value = false
    }
  }

  /** 拖动到某一时刻：拉取历史快照并投喂给画布。 */
  async function seek(at: number) {
    cursor.value = at
    snapshotLoading.value = true
    try {
      const res = await api.snapshotAt(toRFC3339Beijing(at))
      graph.setTopology(res.topology)
      graph.clearFocus()
      graph.selectNode(null)
      snapshotMeta.value = res.replay
      historyMode.value = true
      diff.value = null
    } catch (err) {
      reportError(err, '历史快照回溯失败')
    } finally {
      snapshotLoading.value = false
    }
  }

  /** 退出历史模式，回到实时拓扑。 */
  async function returnToLive() {
    historyMode.value = false
    snapshotMeta.value = null
    diff.value = null
    cursor.value = maxTime.value
    await graph.refreshTopology()
    graph.clearFocus()
  }

  /** 对比「历史时刻」与「当前」。 */
  async function compareWithNow() {
    diffLoading.value = true
    try {
      diff.value = await api.diff(cursorISO.value, toRFC3339Beijing(Date.now()))
      if (diff.value.summary.total_difference === 0) {
        toast.info('该时刻与当前拓扑没有差异')
      }
    } catch (err) {
      reportError(err, '拓扑比对失败')
    } finally {
      diffLoading.value = false
    }
  }

  function clearDiff() {
    diff.value = null
  }

  return {
    bounds,
    events,
    eventTotal,
    eventsLoading,
    cursor,
    cursorISO,
    historyMode,
    snapshotLoading,
    snapshotMeta,
    diff,
    diffLoading,
    minTime,
    maxTime,

    loadBounds,
    loadEvents,
    seek,
    returnToLive,
    compareWithNow,
    clearDiff,
  }
})
