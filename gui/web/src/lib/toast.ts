// Toast notification store
// This provides a reactive store for toast messages that can be used across the app

export type ToastType = 'success' | 'error' | 'warning' | 'info'

export interface ToastMessage {
  id: number
  type: ToastType
  message: string
  duration: number
}

type ToastSubscriber = (toasts: ToastMessage[]) => void

let toasts: ToastMessage[] = []
let nextId = 0
const subscribers = new Set<ToastSubscriber>()

function notify() {
  subscribers.forEach(fn => fn([...toasts]))
}

export function subscribe(fn: ToastSubscriber): () => void {
  subscribers.add(fn)
  fn([...toasts])
  return () => subscribers.delete(fn)
}

function addToast(type: ToastType, message: string, duration = 4000): number {
  const id = nextId++
  toasts = [...toasts, { id, type, message, duration }]
  notify()
  if (duration > 0) {
    setTimeout(() => dismiss(id), duration)
  }
  return id
}

export function dismiss(id: number): void {
  toasts = toasts.filter(t => t.id !== id)
  notify()
}

export function dismissAll(): void {
  toasts = []
  notify()
}

export const toast = {
  success: (message: string, duration?: number) => addToast('success', message, duration),
  error: (message: string, duration?: number) => addToast('error', message, duration ?? 6000),
  warning: (message: string, duration?: number) => addToast('warning', message, duration),
  info: (message: string, duration?: number) => addToast('info', message, duration),
  dismiss,
  dismissAll,
  subscribe,
}
