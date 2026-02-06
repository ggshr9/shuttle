// WebSocket connection utility

export interface WSConnection {
  close(): void
}

export function connectWS<T>(path: string, onMessage: (data: T) => void): WSConnection {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  const url = `${proto}//${location.host}${path}`
  let ws: WebSocket | null = null
  let closed = false

  function connect() {
    if (closed) return
    ws = new WebSocket(url)
    ws.onmessage = (e: MessageEvent) => {
      try {
        onMessage(JSON.parse(e.data) as T)
      } catch {
        // Ignore parse errors
      }
    }
    ws.onclose = () => {
      if (!closed) setTimeout(connect, 2000)
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
