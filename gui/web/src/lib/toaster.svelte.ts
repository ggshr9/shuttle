// Toast store — Svelte 5 runes module.

export type ToastType = 'success' | 'error' | 'warning' | 'info'

export interface ToastMessage {
  id: number
  type: ToastType
  message: string
  duration: number
}

const state = $state<{ items: ToastMessage[] }>({ items: [] })
let nextId = 0

function add(type: ToastType, message: string, duration: number): number {
  const id = nextId++
  state.items = [...state.items, { id, type, message, duration }]
  if (duration > 0) setTimeout(() => dismiss(id), duration)
  return id
}

export function dismiss(id: number): void {
  state.items = state.items.filter(t => t.id !== id)
}

export function dismissAll(): void {
  state.items = []
}

export const toasts = {
  get items(): readonly ToastMessage[] { return state.items },
  success: (m: string, d = 4000) => add('success', m, d),
  error:   (m: string, d = 6000) => add('error', m, d),
  warning: (m: string, d = 4000) => add('warning', m, d),
  info:    (m: string, d = 4000) => add('info', m, d),
  dismiss,
  dismissAll,
}
