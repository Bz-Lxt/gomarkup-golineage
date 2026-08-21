/**
 * 表单校验。
 *
 * 规则与后端 service 层保持一致 —— 前端校验是为了即时反馈，
 * 后端校验才是安全边界，两者都不能省。
 */

import type { PropertyValue } from '@/api/types'

/** 与后端 `internal/service` 中的属性键正则等价。 */
export const PROPERTY_KEY_PATTERN = /^[\p{L}\p{N}_-]{1,64}$/u

/** 权重的业务上下界。声明在此处而非硬编码进校验逻辑，便于统一调整。 */
export const WEIGHT_RANGE = { min: 0.01, max: 1000 } as const

export const NAME_MAX = 128

export interface PropertyRow {
  key: string
  /** 输入框里始终是字符串，保存时再按 type 推断真实类型。 */
  value: string
  type: 'string' | 'number' | 'boolean'
}

export type FieldErrors = Record<string, string>

/**
 * 校验属性键值列表。
 *
 * 返回的 map 以行索引为键，供表单在对应字段下方渲染红色提示。
 */
export function validateProperties(rows: PropertyRow[]): FieldErrors {
  const errors: FieldErrors = {}
  const seen = new Map<string, number>()

  rows.forEach((row, index) => {
    const key = row.key.trim()

    if (!key) {
      errors[`key-${index}`] = '属性名不能为空'
    } else if (!PROPERTY_KEY_PATTERN.test(key)) {
      errors[`key-${index}`] = '仅允许字母、数字、下划线、连字符与中文，长度 1–64'
    } else if (seen.has(key)) {
      errors[`key-${index}`] = `与第 ${seen.get(key)! + 1} 行重名`
    } else {
      seen.set(key, index)
    }

    const value = row.value.trim()
    if (row.type === 'number') {
      if (!value) {
        errors[`value-${index}`] = '数值不能为空'
      } else if (!Number.isFinite(Number(value))) {
        errors[`value-${index}`] = '请输入合法数字'
      }
    } else if (row.type === 'string' && value.length > 512) {
      errors[`value-${index}`] = '文本过长（上限 512 字符）'
    }
  })

  return errors
}

/** 按声明的类型把输入框字符串转为真实的属性值。 */
export function toPropertyValue(row: PropertyRow): PropertyValue {
  const raw = row.value.trim()
  if (row.type === 'number') return Number(raw)
  if (row.type === 'boolean') return raw === 'true' || raw === '是' || raw === '1'
  return raw
}

/** 反向：把后端属性还原为可编辑的行。 */
export function toPropertyRows(properties?: Record<string, PropertyValue>): PropertyRow[] {
  if (!properties) return []
  return Object.entries(properties)
    .sort(([a], [b]) => a.localeCompare(b, 'zh-CN'))
    .map(([key, value]) => ({
      key,
      value: typeof value === 'boolean' ? String(value) : String(value),
      type: typeof value === 'number' ? 'number' : typeof value === 'boolean' ? 'boolean' : 'string',
    }))
}

export function rowsToProperties(rows: PropertyRow[]): Record<string, PropertyValue> {
  const out: Record<string, PropertyValue> = {}
  for (const row of rows) {
    const key = row.key.trim()
    if (!key) continue
    out[key] = toPropertyValue(row)
  }
  return out
}

/** 资产名称校验。 */
export function validateName(name: string): string | null {
  const trimmed = name.trim()
  if (!trimmed) return '资产名称不能为空'
  if (trimmed.length > NAME_MAX) return `名称过长（上限 ${NAME_MAX} 字符）`
  return null
}

/** 关系权重校验，上下界读自 WEIGHT_RANGE 而非写死在这里。 */
export function validateWeight(input: string): string | null {
  const trimmed = input.trim()
  if (!trimmed) return '权重不能为空'
  const n = Number(trimmed)
  if (!Number.isFinite(n)) return '请输入合法数字'
  if (n < WEIGHT_RANGE.min || n > WEIGHT_RANGE.max) {
    return `权重需在 ${WEIGHT_RANGE.min} – ${WEIGHT_RANGE.max} 之间`
  }
  return null
}

export function hasErrors(errors: FieldErrors): boolean {
  return Object.keys(errors).length > 0
}

/** 汇总为一条 Toast 文案。 */
export function summarize(errors: FieldErrors): string {
  const count = Object.keys(errors).length
  const first = Object.values(errors)[0]
  return count > 1 ? `${first}（共 ${count} 处待修正）` : first
}
