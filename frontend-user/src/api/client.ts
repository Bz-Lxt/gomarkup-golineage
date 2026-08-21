/**
 * 后端 API 客户端。
 *
 * 统一处理三件事：响应信封解包、错误归一化、时间参数编码。
 */

import type {
  AllPathsResult,
  CreateEdgeInput,
  CreateNodeInput,
  DeleteNodeResult,
  EventPage,
  GraphEdge,
  GraphNode,
  HealthStatus,
  HistoricalTopology,
  ImpactSummary,
  LineageResult,
  MatrixResult,
  Metadata,
  NodeDetail,
  PathResult,
  SearchResult,
  StructureOverview,
  TimelineBounds,
  Topology,
  TopologyDiff,
  TraverseResult,
  UpdateEdgeInput,
  UpdateNodeInput,
} from './types'

const BASE = '/api/v1'

/** 后端统一响应信封。 */
interface Envelope<T> {
  code: number
  message: string
  data: T
  trace_id: string
}

/**
 * ApiError 携带后端的业务码与链路 ID。
 *
 * 保留 traceId 是为了让用户能在报错提示里看到它 —— 排障时
 * 用这个 ID 去后端日志里一搜即得，远胜于「操作失败」四个字。
 */
export class ApiError extends Error {
  constructor(
    readonly code: number,
    message: string,
    readonly traceId = '',
    readonly httpStatus = 0,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

/** 当前操作者标识，写入请求时随 X-Actor 头一并上送，用于变更流水归属。 */
let actor = 'web-user'
export function setActor(name: string) {
  actor = name.trim() || 'web-user'
}
export function getActor() {
  return actor
}

type Query = Record<string, string | number | boolean | string[] | undefined | null>

/**
 * 拼接查询串。
 *
 * URLSearchParams 会正确编码 RFC3339 时间里的 "+08:00"；
 * 手拼字符串则会让加号在服务端被解码成空格，时间随即失效。
 */
function buildQuery(query?: Query): string {
  if (!query) return ''
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined || value === null || value === '') continue
    if (Array.isArray(value)) {
      for (const item of value) if (item) params.append(key, item)
    } else {
      params.append(key, String(value))
    }
  }
  const qs = params.toString()
  return qs ? `?${qs}` : ''
}

async function request<T>(
  method: string,
  path: string,
  options: { query?: Query; body?: unknown; signal?: AbortSignal } = {},
): Promise<T> {
  const headers: Record<string, string> = { 'X-Actor': actor }
  let payload: string | undefined
  if (options.body !== undefined) {
    headers['Content-Type'] = 'application/json'
    payload = JSON.stringify(options.body)
  }

  let res: Response
  try {
    res = await fetch(`${BASE}${path}${buildQuery(options.query)}`, {
      method,
      headers,
      body: payload,
      signal: options.signal,
    })
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') throw err
    throw new ApiError(-1, '无法连接后端服务，请确认 API 容器已启动')
  }

  let envelope: Envelope<T>
  try {
    envelope = (await res.json()) as Envelope<T>
  } catch {
    throw new ApiError(-2, `服务端返回了非 JSON 响应（HTTP ${res.status}）`, '', res.status)
  }

  if (!res.ok || envelope.code !== 0) {
    throw new ApiError(
      envelope.code ?? res.status,
      envelope.message || `请求失败（HTTP ${res.status}）`,
      envelope.trace_id ?? '',
      res.status,
    )
  }
  return envelope.data
}

export const api = {
  /* ---------------- 元信息 ---------------- */
  metadata: () => request<Metadata>('GET', '/meta'),

  health: async (): Promise<HealthStatus> => {
    const res = await fetch('/healthz')
    const envelope = (await res.json()) as Envelope<HealthStatus>
    if (envelope.code !== 0) throw new ApiError(envelope.code, envelope.message)
    return envelope.data
  },

  /* ---------------- 资产 ---------------- */
  searchNodes: (query: { keyword?: string; type?: string[]; limit?: number }) =>
    request<SearchResult>('GET', '/nodes', { query }),

  getNode: (id: string) => request<NodeDetail>('GET', `/nodes/${encodeURIComponent(id)}`),

  createNode: (input: CreateNodeInput) => request<GraphNode>('POST', '/nodes', { body: input }),

  updateNode: (id: string, input: UpdateNodeInput) =>
    request<GraphNode>('PUT', `/nodes/${encodeURIComponent(id)}`, { body: input }),

  deleteNode: (id: string, reason?: string) =>
    request<DeleteNodeResult>('DELETE', `/nodes/${encodeURIComponent(id)}`, {
      body: { reason, actor: actor },
    }),

  nodeImpact: (id: string, maxDepth?: number) =>
    request<ImpactSummary>('GET', `/nodes/${encodeURIComponent(id)}/impact`, {
      query: { max_depth: maxDepth },
    }),

  nodeEvents: (id: string, limit = 50) =>
    request<EventPage>('GET', `/nodes/${encodeURIComponent(id)}/events`, { query: { limit } }),

  /* ---------------- 关系 ---------------- */
  getEdge: (id: string) => request<GraphEdge>('GET', `/edges/${encodeURIComponent(id)}`),

  createEdge: (input: CreateEdgeInput) => request<GraphEdge>('POST', '/edges', { body: input }),

  updateEdge: (id: string, input: UpdateEdgeInput) =>
    request<GraphEdge>('PUT', `/edges/${encodeURIComponent(id)}`, { body: input }),

  deleteEdge: (id: string, reason?: string) =>
    request<GraphEdge>('DELETE', `/edges/${encodeURIComponent(id)}`, {
      body: { reason, actor: actor },
    }),

  /* ---------------- 图查询 ---------------- */
  topology: (query: { limit?: number; type?: string[]; relation?: string[] } = {}) =>
    request<Topology>('GET', '/graph/', { query }),

  neighbors: (id: string, hops = 1, relation?: string[]) =>
    request<TraverseResult>('GET', '/graph/neighbors', { query: { id, hops, relation } }),

  traverse: (query: {
    algorithm: 'bfs' | 'dfs'
    start: string
    max_depth?: number
    direction?: string
    relation?: string[]
    type?: string[]
    max_nodes?: number
  }) => request<TraverseResult>('GET', '/graph/traverse', { query }),

  shortestPath: (query: {
    from: string
    to: string
    direction?: string
    relation?: string[]
  }) => request<PathResult>('GET', '/graph/shortest-path', { query }),

  allPaths: (query: {
    from: string
    to: string
    direction?: string
    relation?: string[]
    max_depth?: number
    max_paths?: number
  }) => request<AllPathsResult>('GET', '/graph/all-paths', { query }),

  lineage: (query: { root: string; max_depth?: number; relation?: string[]; max_nodes?: number }) =>
    request<LineageResult>('GET', '/graph/lineage', { query }),

  structure: () => request<StructureOverview>('GET', '/graph/structure'),

  matrix: (nodeIds: string[]) =>
    request<MatrixResult>('POST', '/graph/matrix', { body: { node_ids: nodeIds } }),

  /* ---------------- 时间轴 ---------------- */
  events: (query: {
    limit?: number
    offset?: number
    entity_id?: string
    event_type?: string[]
    from?: string
    to?: string
    desc?: boolean
  }) => request<EventPage>('GET', '/timeline/events', { query }),

  snapshotAt: (at: string) => request<HistoricalTopology>('GET', '/timeline/snapshot', {
    query: { at },
  }),

  diff: (from: string, to: string) =>
    request<TopologyDiff>('GET', '/timeline/diff', { query: { from, to } }),

  bounds: () => request<TimelineBounds>('GET', '/timeline/bounds'),
}
