/**
 * 全局消息提示。
 *
 * 硬性要求：支持手动关闭（× 按钮）且 5s 自动消失。
 */
import { readonly, ref } from 'vue'

export type ToastKind = 'success' | 'error' | 'info' | 'warn'

export interface Toast {
  id: number
  kind: ToastKind
  message: string
  /** 后端返回的链路 ID，排障时可直接拿去搜日志。 */
  traceId?: string
}

const AUTO_DISMISS_MS = 5000

const items = ref<Toast[]>([])
const timers = new Map<number, ReturnType<typeof setTimeout>>()
let nextId = 1

function dismiss(id: number) {
  const timer = timers.get(id)
  if (timer) {
    clearTimeout(timer)
    timers.delete(id)
  }
  items.value = items.value.filter((t) => t.id !== id)
}

function push(kind: ToastKind, message: string, traceId?: string) {
  const id = nextId++
  items.value = [...items.value, { id, kind, message, traceId }]
  timers.set(
    id,
    setTimeout(() => dismiss(id), AUTO_DISMISS_MS),
  )
  return id
}

export function useToast() {
  return {
    items: readonly(items),
    dismiss,
    success: (msg: string) => push('success', msg),
    error: (msg: string, traceId?: string) => push('error', msg, traceId),
    warn: (msg: string) => push('warn', msg),
    info: (msg: string) => push('info', msg),
  }
}
