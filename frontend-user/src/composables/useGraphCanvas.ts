/**
 * G6 画布封装。
 *
 * 刻意把 G6 实例放在模块作用域的 shallowRef 里而非 reactive 中：
 * G6 实例内部持有大量循环引用与 canvas 句柄，一旦被 Vue 的深度响应式代理
 * 包裹，既会拖垮性能，也会引发难以定位的渲染异常。
 */
import { Graph } from '@antv/g6'
import { shallowRef } from 'vue'

import type { GraphEdge, GraphNode } from '@/api/types'
import { UI, nodeColor, relationLabel, riskOf, riskStrokeWidth } from '@/utils/palette'

export type LayoutName = 'force' | 'dagre' | 'radial' | 'circular' | 'grid'

export const LAYOUT_OPTIONS: Array<{ value: LayoutName; label: string }> = [
  { value: 'force', label: '力导向' },
  { value: 'dagre', label: '分层' },
  { value: 'radial', label: '辐射' },
  { value: 'circular', label: '环形' },
  { value: 'grid', label: '网格' },
]

/** 节点半径按度数缩放，让枢纽资产在视觉上自然凸显。 */
function sizeByDegree(degree: number): number {
  return Math.min(46, 20 + Math.sqrt(degree) * 6)
}

export interface CanvasCallbacks {
  onNodeClick: (id: string) => void
  onEdgeClick: (id: string) => void
  onCanvasClick: () => void
  onNodeDoubleClick: (id: string) => void
}

function layoutConfig(name: LayoutName) {
  switch (name) {
    case 'dagre':
      return { type: 'antv-dagre', rankdir: 'TB', nodesep: 40, ranksep: 70 }
    case 'radial':
      return { type: 'radial', unitRadius: 110, linkDistance: 160, preventOverlap: true }
    case 'circular':
      return { type: 'circular', radius: 260 }
    case 'grid':
      return { type: 'grid', rows: undefined, cols: undefined, preventOverlap: true }
    case 'force':
    default:
      return {
        type: 'd3-force',
        link: { distance: 140, strength: 0.4 },
        manyBody: { strength: -420 },
        collide: { radius: 42 },
      }
  }
}

export function useGraphCanvas() {
  const instance = shallowRef<Graph | null>(null)

  function buildData(nodes: GraphNode[], edges: GraphEdge[]) {
    const degree = new Map<string, number>()
    for (const e of edges) {
      degree.set(e.source_id, (degree.get(e.source_id) ?? 0) + 1)
      degree.set(e.target_id, (degree.get(e.target_id) ?? 0) + 1)
    }
    return {
      nodes: nodes.map((n) => {
        const risk = riskOf(n.properties)
        return {
          id: n.id,
          data: {
            name: n.name,
            type: n.type,
            degree: degree.get(n.id) ?? 0,
            risk: risk ?? '',
            color: nodeColor(n.type),
            size: sizeByDegree(degree.get(n.id) ?? 0),
            riskWidth: riskStrokeWidth(risk),
          },
        }
      }),
      edges: edges.map((e) => ({
        id: e.id,
        source: e.source_id,
        target: e.target_id,
        data: { relation: e.relation, label: relationLabel(e.relation), weight: e.weight },
      })),
    }
  }

  function create(container: HTMLElement, layout: LayoutName, callbacks: CanvasCallbacks) {
    const graph = new Graph({
      container,
      autoResize: true,
      autoFit: 'view',
      animation: false,
      padding: 24,
      layout: layoutConfig(layout),
      behaviors: [
        'drag-canvas',
        'zoom-canvas',
        // drag-element 即节点拖拽，用户明确要求的能力之一。
        'drag-element',
      ],
      node: {
        style: {
          size: (d: any) => d.data.size,
          fill: (d: any) => d.data.color,
          fillOpacity: 0.92,
          stroke: (d: any) => (d.data.riskWidth > 0 ? riskStroke(d.data.risk) : d.data.color),
          lineWidth: (d: any) => (d.data.riskWidth > 0 ? d.data.riskWidth : 1.6),
          labelText: (d: any) => d.data.name,
          labelFill: UI.ink,
          labelFontSize: 11,
          labelFontFamily: "'IBM Plex Sans', 'PingFang SC', sans-serif",
          labelPlacement: 'bottom',
          labelOffsetY: 4,
          labelBackground: true,
          labelBackgroundFill: UI.void,
          labelBackgroundFillOpacity: 0.75,
          labelBackgroundRadius: 2,
          labelBackgroundPadding: [1, 4, 1, 4],
          cursor: 'pointer',
        },
        state: {
          // 淡出而非隐藏：保留整体轮廓，用户仍能感知全局规模。
          inactive: {
            fillOpacity: 0.12,
            strokeOpacity: 0.2,
            labelOpacity: 0.2,
          },
          highlight: {
            fillOpacity: 1,
            lineWidth: 2.5,
            labelFontSize: 12,
          },
          selected: {
            fillOpacity: 1,
            stroke: UI.signal,
            lineWidth: 3,
            labelFill: UI.signal,
            labelFontSize: 12,
            shadowColor: UI.signal,
            shadowBlur: 24,
          },
          onpath: {
            fillOpacity: 1,
            stroke: UI.amber,
            lineWidth: 2.5,
            labelFill: UI.amber,
          },
        },
      },
      edge: {
        style: {
          stroke: UI.lineStrong,
          lineWidth: 1,
          endArrow: true,
          endArrowSize: 6,
          endArrowFill: UI.lineStrong,
          cursor: 'pointer',
        },
        state: {
          inactive: { strokeOpacity: 0.08, labelOpacity: 0 },
          highlight: { stroke: UI.signal, lineWidth: 1.8, endArrowFill: UI.signal },
          onpath: {
            stroke: UI.amber,
            lineWidth: 2.5,
            endArrowFill: UI.amber,
            labelText: (d: any) => d.data.label,
            labelFill: UI.amber,
            labelFontSize: 10,
          },
          selected: { stroke: UI.signal, lineWidth: 2.5, endArrowFill: UI.signal },
        },
      },
    })

    graph.on('node:click', (event: any) => callbacks.onNodeClick(String(event.target.id)))
    graph.on('node:dblclick', (event: any) => callbacks.onNodeDoubleClick(String(event.target.id)))
    graph.on('edge:click', (event: any) => callbacks.onEdgeClick(String(event.target.id)))
    graph.on('canvas:click', () => callbacks.onCanvasClick())

    instance.value = graph
    return graph
  }

  function riskStroke(risk: string): string {
    if (risk === 'critical') return UI.danger
    if (risk === 'high') return UI.amber
    if (risk === 'medium') return '#F6BD16'
    return UI.lineStrong
  }

  /**
   * 应用高亮状态。
   *
   * 走 setElementState 而非重设数据：后者会触发重新布局，
   * 节点位置突变会让用户彻底失去空间参照。
   */
  function applyStates(
    nodes: GraphNode[],
    edges: GraphEdge[],
    opts: {
      focusNodes: Set<string>
      focusEdges: Set<string>
      selectedNodeId: string | null
      selectedEdgeId: string | null
      pathMode: boolean
      hiddenNodes: Set<string> | null
    },
  ) {
    const graph = instance.value
    if (!graph) return

    const states: Record<string, string[]> = {}
    const dimAll = opts.focusNodes.size > 0 || opts.hiddenNodes !== null

    for (const n of nodes) {
      const filteredOut = opts.hiddenNodes !== null && !opts.hiddenNodes.has(n.id)
      const focused = opts.focusNodes.size === 0 || opts.focusNodes.has(n.id)

      if (filteredOut || (dimAll && !focused)) {
        states[n.id] = ['inactive']
      } else if (n.id === opts.selectedNodeId) {
        states[n.id] = ['selected']
      } else if (opts.pathMode && opts.focusNodes.has(n.id)) {
        states[n.id] = ['onpath']
      } else if (opts.focusNodes.has(n.id)) {
        states[n.id] = ['highlight']
      } else {
        states[n.id] = []
      }
    }

    for (const e of edges) {
      const endpointHidden =
        opts.hiddenNodes !== null &&
        (!opts.hiddenNodes.has(e.source_id) || !opts.hiddenNodes.has(e.target_id))
      const focused = opts.focusEdges.size === 0 || opts.focusEdges.has(e.id)

      if (endpointHidden || (dimAll && !focused)) {
        states[e.id] = ['inactive']
      } else if (e.id === opts.selectedEdgeId) {
        states[e.id] = ['selected']
      } else if (opts.pathMode && opts.focusEdges.has(e.id)) {
        states[e.id] = ['onpath']
      } else if (opts.focusEdges.has(e.id)) {
        states[e.id] = ['highlight']
      } else {
        states[e.id] = []
      }
    }

    graph.setElementState(states)
  }

  function destroy() {
    instance.value?.destroy()
    instance.value = null
  }

  return { instance, create, buildData, applyStates, destroy, layoutConfig }
}
