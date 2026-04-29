// WebSocket connection utility

import { getAuthToken } from './api/client'
import { nextBackoffMs } from './backoff'

export interface WSConnection {
  close(): void
}

export function connectWS<T>(path: string, onMessage: (data: T) => void): WSConnection {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  const baseUrl = `${proto}//${location.host}${path}`
  let ws: WebSocket | null = null
  let closed = false
  let attempt = 0

  function connect() {
    if (closed) return
    const token = getAuthToken()
    const url = token ? `${baseUrl}?token=${encodeURIComponent(token)}` : baseUrl
    ws = new WebSocket(url)
    ws.onopen = () => {
      attempt = 0 // success resets the backoff window
    }
    ws.onmessage = (e: MessageEvent) => {
      try {
        onMessage(JSON.parse(e.data) as T)
      } catch {
        // Ignore parse errors
      }
    }
    ws.onclose = () => {
      if (closed) return
      const delay = nextBackoffMs(attempt)
      attempt++
      setTimeout(connect, delay)
    }
    ws.onerror = () => ws?.close()
  }

  connect()

  return {
    close() {
      closed = true
      if (ws) ws.close()
    },
  }
}
