export function connectWS(path, onMessage) {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  const url = `${proto}//${location.host}${path}`
  let ws = null
  let closed = false

  function connect() {
    if (closed) return
    ws = new WebSocket(url)
    ws.onmessage = (e) => {
      try {
        onMessage(JSON.parse(e.data))
      } catch {}
    }
    ws.onclose = () => {
      if (!closed) setTimeout(connect, 2000)
    }
    ws.onerror = () => ws.close()
  }

  connect()

  return {
    close() {
      closed = true
      if (ws) ws.close()
    },
  }
}
