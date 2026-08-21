/**
 * 视觉编码字典。
 *
 * 资产类型色在图例、画布、列表、抽屉、事件流中全局复用，
 * 用户由此建立「绿色=数据库」这样的肌肉记忆。任何一处不一致都会破坏它。
 */

import type { NodeType, RelationType } from '@/api/types'

export const NODE_COLORS: Record<NodeType, string> = {
  server: '#5B8FF9',
  database: '#5AD8A6',
  application: '#F6BD16',
  api: '#E8684A',
  person: '#6DC8EC',
  account: '#9270CA',
  middleware: '#FF9D4D',
  storage: '#269A99',
}

export const NODE_LABELS: Record<NodeType, string> = {
  server: '服务器',
  database: '数据库',
  application: '应用',
  api: '接口',
  person: '人员',
  account: '账户',
  middleware: '中间件',
  storage: '存储',
}

export const RELATION_LABELS: Record<RelationType, string> = {
  deploys_on: '部署于',
  calls: '调用',
  reads_from: '读取',
  writes_to: '写入',
  depends_on: '依赖',
  owns: '拥有',
  transfers_to: '转账至',
  associates_with: '关联',
}

export const EVENT_LABELS: Record<string, string> = {
  node_created: '新增资产',
  node_updated: '修改资产',
  node_deleted: '删除资产',
  edge_created: '建立关系',
  edge_updated: '修改关系',
  edge_deleted: '解除关系',
}

/** 事件类型 → 语义色。新增用青、修改用琥珀、删除用红。 */
export const EVENT_COLORS: Record<string, string> = {
  node_created: '#2DD4BF',
  edge_created: '#2DD4BF',
  node_updated: '#F5A524',
  edge_updated: '#F5A524',
  node_deleted: '#F2555A',
  edge_deleted: '#F2555A',
}

export const UI = {
  signal: '#2DD4BF',
  amber: '#F5A524',
  danger: '#F2555A',
  violet: '#A78BFA',
  ink: '#E6EAF2',
  inkDim: '#8B95A8',
  inkMute: '#5A6577',
  line: '#222A38',
  lineStrong: '#2E3A4D',
  panel: '#0E121A',
  elevated: '#151A24',
  void: '#07090E',
} as const

export function nodeColor(type: string): string {
  return NODE_COLORS[type as NodeType] ?? UI.inkDim
}

export function nodeLabel(type: string): string {
  return NODE_LABELS[type as NodeType] ?? type
}

export function relationLabel(rel: string): string {
  return RELATION_LABELS[rel as RelationType] ?? rel
}

export function eventLabel(t: string): string {
  return EVENT_LABELS[t] ?? t
}

export function eventColor(t: string): string {
  return EVENT_COLORS[t] ?? UI.inkDim
}

/* ---------------- 风险等级 ---------------- */

export type RiskLevel = 'critical' | 'high' | 'medium' | 'low'

export const RISK_COLORS: Record<RiskLevel, string> = {
  critical: '#F2555A',
  high: '#F5A524',
  medium: '#F6BD16',
  low: '#5AD8A6',
}

export const RISK_LABELS: Record<RiskLevel, string> = {
  critical: '严重',
  high: '高',
  medium: '中',
  low: '低',
}

/**
 * 从节点属性中提取风险等级。
 *
 * 属性由用户自由添加，键名大小写不可控，值也可能是「高」「HIGH」等写法，
 * 因此这里做宽松归一化而不是精确匹配。
 */
export function riskOf(properties?: Record<string, unknown>): RiskLevel | null {
  if (!properties) return null
  for (const [key, value] of Object.entries(properties)) {
    const k = key.toLowerCase()
    if (k !== 'risk_level' && k !== 'risk' && k !== '风险等级' && k !== '风险') continue
    const v = String(value).toLowerCase().trim()
    if (v === 'critical' || v === '严重' || v === '致命') return 'critical'
    if (v === 'high' || v === '高') return 'high'
    if (v === 'medium' || v === 'mid' || v === '中') return 'medium'
    if (v === 'low' || v === '低') return 'low'
  }
  return null
}

/** 风险等级对应的描边宽度。等级越高环越粗，无风险则不描边。 */
export function riskStrokeWidth(level: RiskLevel | null): number {
  if (level === 'critical') return 3
  if (level === 'high') return 3
  if (level === 'medium') return 2
  return 0
}
