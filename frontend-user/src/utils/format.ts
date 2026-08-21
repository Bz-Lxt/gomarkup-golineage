/**
 * 展示层格式化。
 *
 * 用户可见的一切时间统一为 `yyyy-MM-dd HH:mm:ss`（北京时间）；
 * 传输层仍保持 ISO 8601 / RFC3339。
 */

const BEIJING_OFFSET_MS = 8 * 60 * 60 * 1000

function pad(n: number, width = 2): string {
  return String(n).padStart(width, '0')
}

/** 把任意时间输入转为北京时区的各字段。 */
function beijingParts(input: string | number | Date) {
  const date = input instanceof Date ? input : new Date(input)
  if (Number.isNaN(date.getTime())) return null
  // 统一用 UTC 取值再手动加 8 小时，避免依赖浏览器本地时区。
  const shifted = new Date(date.getTime() + BEIJING_OFFSET_MS)
  return {
    y: shifted.getUTCFullYear(),
    M: shifted.getUTCMonth() + 1,
    d: shifted.getUTCDate(),
    h: shifted.getUTCHours(),
    m: shifted.getUTCMinutes(),
    s: shifted.getUTCSeconds(),
  }
}

/** `yyyy-MM-dd HH:mm:ss` —— 全站用户可见时间的唯一格式。 */
export function formatDateTime(input?: string | number | Date | null): string {
  if (!input) return '—'
  const p = beijingParts(input)
  if (!p) return '—'
  return `${p.y}-${pad(p.M)}-${pad(p.d)} ${pad(p.h)}:${pad(p.m)}:${pad(p.s)}`
}

/** `MM-dd HH:mm` —— 时间轴刻度等空间紧张处使用。 */
export function formatShort(input?: string | number | Date | null): string {
  if (!input) return '—'
  const p = beijingParts(input)
  if (!p) return '—'
  return `${pad(p.M)}-${pad(p.d)} ${pad(p.h)}:${pad(p.m)}`
}

/** `yyyy-MM-dd` */
export function formatDate(input?: string | number | Date | null): string {
  if (!input) return '—'
  const p = beijingParts(input)
  if (!p) return '—'
  return `${p.y}-${pad(p.M)}-${pad(p.d)}`
}

/**
 * 转为带 +08:00 偏移的 RFC3339，用于回传后端。
 *
 * 不用 toISOString()：那会输出 UTC 的 Z 后缀，虽然时刻等价，
 * 但后端日志与前端展示对不上，排障时得心算 8 小时。
 */
export function toRFC3339Beijing(input: number | Date): string {
  const p = beijingParts(input)
  if (!p) return ''
  return `${p.y}-${pad(p.M)}-${pad(p.d)}T${pad(p.h)}:${pad(p.m)}:${pad(p.s)}+08:00`
}

/** 相对时间，用于事件流。 */
export function formatRelative(input?: string | null): string {
  if (!input) return '—'
  const t = new Date(input).getTime()
  if (Number.isNaN(t)) return '—'
  const diff = Date.now() - t
  if (diff < 0) return '刚刚'
  const min = Math.floor(diff / 60000)
  if (min < 1) return '刚刚'
  if (min < 60) return `${min} 分钟前`
  const hour = Math.floor(min / 60)
  if (hour < 24) return `${hour} 小时前`
  const day = Math.floor(hour / 24)
  if (day < 30) return `${day} 天前`
  return formatDate(input)
}

/** 大数字加千分位。 */
export function formatNumber(n: number | undefined | null): string {
  if (n === undefined || n === null || Number.isNaN(n)) return '—'
  return n.toLocaleString('zh-CN')
}

/** 保留指定小数位，去掉无意义的尾随零。 */
export function formatDecimal(n: number | undefined | null, digits = 2): string {
  if (n === undefined || n === null || Number.isNaN(n)) return '—'
  return String(Number(n.toFixed(digits)))
}

/** 属性值转为可展示文本。 */
export function formatPropertyValue(v: unknown): string {
  if (v === null || v === undefined) return '—'
  if (typeof v === 'boolean') return v ? '是' : '否'
  if (typeof v === 'number') return formatDecimal(v, 4)
  return String(v)
}

/** 超长 ID 中段省略，避免撑破布局。 */
export function truncateMiddle(text: string, max = 24): string {
  if (text.length <= max) return text
  const head = Math.ceil((max - 1) / 2)
  const tail = Math.floor((max - 1) / 2)
  return `${text.slice(0, head)}…${text.slice(-tail)}`
}
