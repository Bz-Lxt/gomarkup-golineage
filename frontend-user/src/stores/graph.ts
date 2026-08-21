/**
 * 图谱主状态。
 *
 * 承担三件事：持有当前画布数据、维护高亮语义、集中处理写操作后的刷新。
 */
import { defineStore } from 'pinia'
import { computed, ref, shallowRef } from 'vue'

import { ApiError, api } from '@/api/client'
import type {
  AllPathsResult,
  CreateEdgeInput,
  CreateNodeInput,
  GraphEdge,
  GraphNode,
  LineageResult,
  Metadata,
  NodeDetail,
  PathResult,
  Topology,
  UpdateNodeInput,
} from '@/api/types'
import { useToast } from '@/composables/useToast'

/**
 * 画布的高亮语义。
 * 同一时刻只能有一种，否则用户无法判断「暗下去的节点」到底是被哪条规则排除的。
 */
export type FocusMode = 'none' | 'neighbors' | 'path' | 'lineage'

export const useGraphStore = defineStore('graph', () => {
  const toast = useToast()

  /* ---------------- 基础数据 ---------------- */
  const metadata = ref<Metadata | null>(null)
  const topology = shallowRef<Topology | null>(null)
  const loading = ref(false)
  const initError = ref('')

  /* ---------------- 选中与高亮 ---------------- */
  const selectedNodeId = ref<string | null>(null)
  const selectedEdgeId = ref<string | null>(null)
  const nodeDetail = ref<NodeDetail | null>(null)
  const detailLoading = ref(false)

  const focusMode = ref<FocusMode>('none')
  /** 高亮集合。为空表示不做淡出处理。 */
  const focusNodes = ref<Set<string>>(new Set())
  const focusEdges = ref<Set<string>>(new Set())

  /* ---------------- 路径分析 ---------------- */
  const pathFrom = ref('')
  const pathTo = ref('')
  const pathResult = ref<PathResult | null>(null)
  const allPathsResult = ref<AllPathsResult | null>(null)
  const pathLoading = ref(false)

  /* ---------------- 血缘分析 ---------------- */
  const lineageResult = ref<LineageResult | null>(null)
  const lineageLoading = ref(false)

  /* ---------------- 过滤 ---------------- */
  const typeFilter = ref<Set<string>>(new Set())

  /* ---------------- 派生 ---------------- */
  const nodes = computed<GraphNode[]>(() => topology.value?.nodes ?? [])
  const edges = computed<GraphEdge[]>(() => topology.value?.edges ?? [])
  const stats = computed(() => topology.value?.stats ?? null)

  const nodeMap = computed(() => {
    const map = new Map<string, GraphNode>()
    for (const n of nodes.value) map.set(n.id, n)
    return map
  })

  const selectedNode = computed(() =>
    selectedNodeId.value ? (nodeMap.value.get(selectedNodeId.value) ?? null) : null,
  )

  const selectedEdge = computed(() =>
    selectedEdgeId.value ? (edges.value.find((e) => e.id === selectedEdgeId.value) ?? null) : null,
  )

  /** 供下拉框使用的资产选项，按名称排序。 */
  const nodeOptions = computed(() =>
    [...nodes.value]
      .sort((a, b) => a.name.localeCompare(b.name, 'zh-CN'))
      .map((n) => ({ value: n.id, label: `${n.name}` })),
  )

  function reportError(err: unknown, fallback: string) {
    if (err instanceof ApiError) {
      toast.error(err.message || fallback, err.traceId)
    } else if (err instanceof DOMException && err.name === 'AbortError') {
      return
    } else {
      toast.error(fallback)
    }
  }

  /* ---------------- 载入 ---------------- */

  async function bootstrap() {
    loading.value = true
    initError.value = ''
    try {
      const [meta, topo] = await Promise.all([api.metadata(), api.topology({ limit: 2000 })])
      metadata.value = meta
      topology.value = topo
      if (topo.truncated) {
        toast.warn(`拓扑规模超出单次上限，已展示前 ${topo.nodes.length} 个资产`)
      }
    } catch (err) {
      initError.value = err instanceof ApiError ? err.message : '初始化失败'
      reportError(err, '初始化失败')
    } finally {
      loading.value = false
    }
  }

  async function refreshTopology() {
    try {
      topology.value = await api.topology({ limit: 2000 })
    } catch (err) {
      reportError(err, '刷新拓扑失败')
    }
  }

  /** 外部（时间轴回溯）直接投喂拓扑，不经过接口。 */
  function setTopology(topo: Topology | null) {
    topology.value = topo
  }

  /* ---------------- 选中 ---------------- */

  async function selectNode(id: string | null) {
    selectedEdgeId.value = null
    selectedNodeId.value = id
    nodeDetail.value = null
    if (!id) {
      clearFocus()
      return
    }
    detailLoading.value = true
    try {
      nodeDetail.value = await api.getNode(id)
      highlightNeighbors(id)
    } catch (err) {
      reportError(err, '读取资产详情失败')
    } finally {
      detailLoading.value = false
    }
  }

  function selectEdge(id: string | null) {
    selectedNodeId.value = null
    nodeDetail.value = null
    selectedEdgeId.value = id
    clearFocus()
  }

  /* ---------------- 高亮 ---------------- */

  /** 点击节点 → 一跳邻居保持原色，其余淡出。 */
  function highlightNeighbors(id: string) {
    const keepNodes = new Set<string>([id])
    const keepEdges = new Set<string>()
    for (const edge of edges.value) {
      if (edge.source_id === id) {
        keepNodes.add(edge.target_id)
        keepEdges.add(edge.id)
      } else if (edge.target_id === id) {
        keepNodes.add(edge.source_id)
        keepEdges.add(edge.id)
      }
    }
    focusNodes.value = keepNodes
    focusEdges.value = keepEdges
    focusMode.value = 'neighbors'
  }

  function clearFocus() {
    focusNodes.value = new Set()
    focusEdges.value = new Set()
    focusMode.value = 'none'
    pathResult.value = null
    allPathsResult.value = null
    lineageResult.value = null
  }

  /* ---------------- 路径 ---------------- */

  async function findShortestPath() {
    if (!pathFrom.value || !pathTo.value) {
      toast.warn('请先选择起点与终点')
      return
    }
    if (pathFrom.value === pathTo.value) {
      toast.warn('起点与终点不能相同')
      return
    }
    pathLoading.value = true
    allPathsResult.value = null
    try {
      const res = await api.shortestPath({
        from: pathFrom.value,
        to: pathTo.value,
        direction: 'out',
      })
      pathResult.value = res
      if (!res.found) {
        focusMode.value = 'none'
        focusNodes.value = new Set()
        focusEdges.value = new Set()
        toast.warn('两个资产之间不存在可达路径')
        return
      }
      focusNodes.value = new Set((res.nodes ?? []).map((n) => n.id))
      focusEdges.value = new Set((res.edges ?? []).map((e) => e.id))
      focusMode.value = 'path'
      selectedNodeId.value = null
      nodeDetail.value = null
      toast.success(`找到最短路径：${res.hops} 跳，总代价 ${res.total_cost}`)
    } catch (err) {
      reportError(err, '最短路径查询失败')
    } finally {
      pathLoading.value = false
    }
  }

  async function findAllPaths() {
    if (!pathFrom.value || !pathTo.value) {
      toast.warn('请先选择起点与终点')
      return
    }
    if (pathFrom.value === pathTo.value) {
      toast.warn('起点与终点不能相同')
      return
    }
    pathLoading.value = true
    pathResult.value = null
    try {
      const res = await api.allPaths({
        from: pathFrom.value,
        to: pathTo.value,
        direction: 'out',
        max_depth: metadata.value?.limits.max_depth ?? 10,
      })
      allPathsResult.value = res
      if (res.paths.length === 0) {
        focusMode.value = 'none'
        toast.warn('两个资产之间不存在可达路径')
        return
      }
      focusNodes.value = new Set(res.nodes.map((n) => n.id))
      focusEdges.value = new Set(res.edges.map((e) => e.id))
      focusMode.value = 'path'
      selectedNodeId.value = null
      nodeDetail.value = null
      const suffix = res.truncated ? '（已达上限，可能还有更多）' : ''
      toast.success(`共找到 ${res.paths.length} 条路径${suffix}`)
    } catch (err) {
      reportError(err, '全路径查询失败')
    } finally {
      pathLoading.value = false
    }
  }

  /* ---------------- 血缘 ---------------- */

  async function analyzeLineage(rootId: string) {
    lineageLoading.value = true
    try {
      const res = await api.lineage({
        root: rootId,
        max_depth: metadata.value?.limits.max_depth ?? 10,
      })
      lineageResult.value = res
      focusNodes.value = new Set(res.nodes.map((n) => n.id))
      focusEdges.value = new Set(res.edges.map((e) => e.id))
      focusMode.value = 'lineage'
      toast.success(`上游 ${res.upstream_count} 个，下游 ${res.downstream_count} 个`)
    } catch (err) {
      reportError(err, '血缘分析失败')
    } finally {
      lineageLoading.value = false
    }
  }

  /* ---------------- 过滤 ---------------- */

  function toggleTypeFilter(type: string) {
    const next = new Set(typeFilter.value)
    if (next.has(type)) next.delete(type)
    else next.add(type)
    typeFilter.value = next
  }

  function resetTypeFilter() {
    typeFilter.value = new Set()
  }

  /* ---------------- 写入 ---------------- */

  async function createNode(input: CreateNodeInput) {
    const node = await api.createNode(input)
    await refreshTopology()
    toast.success(`已新增资产「${node.name}」`)
    await selectNode(node.id)
    return node
  }

  async function updateNode(id: string, input: UpdateNodeInput) {
    const node = await api.updateNode(id, input)
    await refreshTopology()
    toast.success(`已更新「${node.name}」`)
    await selectNode(id)
    return node
  }

  async function removeNode(id: string, reason?: string) {
    const res = await api.deleteNode(id, reason)
    const cascaded = res.cascaded_edges?.length ?? 0
    await refreshTopology()
    selectNode(null)
    clearFocus()
    toast.success(
      cascaded > 0
        ? `已删除「${res.node.name}」，并级联解除 ${cascaded} 条关系`
        : `已删除「${res.node.name}」`,
    )
    return res
  }

  async function createEdge(input: CreateEdgeInput) {
    const edge = await api.createEdge(input)
    await refreshTopology()
    toast.success('已建立血缘关系')
    selectEdge(edge.id)
    return edge
  }

  async function removeEdge(id: string, reason?: string) {
    const edge = await api.deleteEdge(id, reason)
    await refreshTopology()
    selectEdge(null)
    clearFocus()
    toast.success(`已解除关系 ${edge.id}`)
    return edge
  }

  /** 类型过滤后的可见节点集合。空集合表示不过滤。 */
  const visibleNodeIds = computed(() => {
    if (typeFilter.value.size === 0) return null
    const ids = new Set<string>()
    for (const n of nodes.value) if (typeFilter.value.has(n.type)) ids.add(n.id)
    return ids
  })

  return {
    metadata,
    topology,
    loading,
    initError,
    nodes,
    edges,
    stats,
    nodeMap,
    nodeOptions,

    selectedNodeId,
    selectedEdgeId,
    selectedNode,
    selectedEdge,
    nodeDetail,
    detailLoading,

    focusMode,
    focusNodes,
    focusEdges,

    pathFrom,
    pathTo,
    pathResult,
    allPathsResult,
    pathLoading,

    lineageResult,
    lineageLoading,

    typeFilter,
    visibleNodeIds,

    bootstrap,
    refreshTopology,
    setTopology,
    selectNode,
    selectEdge,
    highlightNeighbors,
    clearFocus,
    findShortestPath,
    findAllPaths,
    analyzeLineage,
    toggleTypeFilter,
    resetTypeFilter,
    createNode,
    updateNode,
    removeNode,
    createEdge,
    removeEdge,
    reportError,
  }
})
