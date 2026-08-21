/**
 * 与后端 DTO 一一对应的类型定义。
 *
 * 字段名严格照抄 Go 结构体的 json tag —— 这里每错一个字段名，
 * 都只会在运行时表现为静默的 undefined，而不是编译错误。
 */

export type NodeType =
  | 'server'
  | 'database'
  | 'application'
  | 'api'
  | 'person'
  | 'account'
  | 'middleware'
  | 'storage'

export type RelationType =
  | 'deploys_on'
  | 'calls'
  | 'reads_from'
  | 'writes_to'
  | 'depends_on'
  | 'owns'
  | 'transfers_to'
  | 'associates_with'

export type PropertyValue = string | number | boolean
export type Properties = Record<string, PropertyValue>

export interface GraphNode {
  id: string
  name: string
  type: NodeType
  properties?: Properties
  created_at: string
  updated_at: string
}

export interface GraphEdge {
  id: string
  source_id: string
  target_id: string
  relation: RelationType
  weight: number
  directed: boolean
  properties?: Properties
  created_at: string
  updated_at: string
}

export interface GraphStats {
  node_count: number
  edge_count: number
  type_counts: Record<string, number>
  avg_degree: number
  max_degree: number
  isolated_count: number
}

export interface Topology {
  nodes: GraphNode[]
  edges: GraphEdge[]
  total_nodes: number
  total_edges: number
  truncated: boolean
  stats: GraphStats
}

export interface SearchResult {
  items: GraphNode[]
  count: number
}

export interface ImpactSummary {
  node_id: string
  node_name: string
  direct_downstream: number
  total_downstream: number
  direct_upstream: number
  total_upstream: number
  downstream_by_type: Record<string, number>
  max_depth_reached: number
}

export interface NodeDetail {
  node: GraphNode
  in_degree: number
  out_degree: number
  impact: ImpactSummary
  incident_edges: GraphEdge[]
  event_count: number
}

export interface PathResult {
  found: boolean
  nodes: GraphNode[] | null
  edges: GraphEdge[] | null
  total_cost: number
  hops: number
  visited_count: number
  truncated: boolean
}

export interface AllPathsResult {
  paths: PathResult[]
  truncated: boolean
  explored_edges: number
  nodes: GraphNode[]
  edges: GraphEdge[]
}

export interface TraverseResult {
  algorithm: string
  nodes: GraphNode[]
  edges: GraphEdge[]
  depths: Record<string, number>
  order: string[]
  truncated: boolean
  cycle_detected: boolean
  visited_count: number
}

export interface LineageResult {
  root: GraphNode
  upstream: GraphNode[]
  downstream: GraphNode[]
  nodes: GraphNode[]
  edges: GraphEdge[]
  /** 负数为上游层级，正数为下游层级，0 为根节点自身。 */
  levels: Record<string, number>
  upstream_count: number
  downstream_count: number
  truncated: boolean
}

export interface StructureOverview {
  stats: GraphStats
  roots: GraphNode[]
  leaves: GraphNode[]
  has_cycle: boolean
  cycle_hint?: string
  topological_length: number
}

export interface MatrixResult {
  node_ids: string[]
  node_names: string[]
  matrix: number[][]
  two_hop_reach: number[][]
  density: number
  components: string[][]
  component_count: number
  largest_component: number
  edge_count: number
}

export type EventType =
  | 'node_created'
  | 'node_updated'
  | 'node_deleted'
  | 'edge_created'
  | 'edge_updated'
  | 'edge_deleted'

export interface LineageEvent {
  seq: number
  event_type: EventType
  event_label: string
  entity_type: 'node' | 'edge'
  entity_id: string
  payload?: Record<string, unknown>
  before?: Record<string, unknown>
  actor: string
  reason: string
  occurred_at: string
}

export interface EventPage {
  items: LineageEvent[]
  total: number
  limit: number
  offset: number
}

export interface ReplayMeta {
  events_applied: number
  last_seq: number
  duration_ms: number
}

export interface HistoricalTopology {
  at: string
  topology: Topology
  replay: ReplayMeta
}

export interface FieldChange {
  field: string
  before: unknown
  after: unknown
}

export interface NodeChange {
  id: string
  name: string
  type: string
  changes: FieldChange[]
}

export interface EdgeChange {
  id: string
  source_id: string
  target_id: string
  relation: string
  changes: FieldChange[]
}

export interface DiffSummary {
  nodes_added: number
  nodes_removed: number
  nodes_modified: number
  edges_added: number
  edges_removed: number
  edges_modified: number
  total_difference: number
}

export interface TopologyDiff {
  from: string
  to: string
  added_nodes: GraphNode[]
  removed_nodes: GraphNode[]
  modified_nodes: NodeChange[]
  added_edges: GraphEdge[]
  removed_edges: GraphEdge[]
  modified_edges: EdgeChange[]
  summary: DiffSummary
}

export interface TimelineBounds {
  earliest: string
  latest: string
  event_count: number
  available: boolean
}

export interface EnumOption {
  value: string
  label: string
}

export interface Limits {
  max_depth: number
  max_paths: number
  max_nodes: number
}

export interface Metadata {
  node_types: NodeType[]
  relation_types: RelationType[]
  event_types: EnumOption[]
  property_keys: string[]
  limits: Limits
  stats: GraphStats
  adapter: string
  server_time: string
}

export interface HealthStatus {
  status: string
  database: string
  adapter: string
  node_count: number
  edge_count: number
  event_count: number
  server_time: string
}

export interface DeleteNodeResult {
  node: GraphNode
  cascaded_edges: GraphEdge[]
}

/* ---------------- 写入请求体 ---------------- */

export interface CreateNodeInput {
  name: string
  type: NodeType
  properties?: Properties
  actor?: string
  reason?: string
}

export interface UpdateNodeInput {
  name?: string
  type?: NodeType
  properties?: Properties
  replace_properties?: boolean
  actor?: string
  reason?: string
}

export interface CreateEdgeInput {
  source_id: string
  target_id: string
  relation: RelationType
  weight?: number
  directed?: boolean
  properties?: Properties
  actor?: string
  reason?: string
}

export interface UpdateEdgeInput {
  relation?: RelationType
  weight?: number
  directed?: boolean
  properties?: Properties
  replace_properties?: boolean
  actor?: string
  reason?: string
}
