// gui/web/src/lib/data/http-subscription.ts
import { SubscriptionBase } from './subscription-base'
import type { TopicKey, TopicKind } from './topics'
import type { ConnectionStateController } from './connection-state'

export class HttpSubscription<T> extends SubscriptionBase<T> {
  private ws: WebSocket | null = null
  private closed = false

  constructor(
    topic: TopicKey,
    kind: TopicKind,
    private readonly wsPath: string,
    private readonly conn: ConnectionStateController,
    private readonly authToken: () => string = () => '',
  ) {
    super(topic, kind)
  }

  protected connect(): void {
    if (this.ws) return
    this.closed = false
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const tok = this.authToken()
    const qs = tok ? `?token=${encodeURIComponent(tok)}` : ''
    const url = `${proto}//${location.host}${this.wsPath}${qs}`
    const ws = new WebSocket(url)
    this.ws = ws
    ws.onmessage = (ev: MessageEvent) => {
      try {
        const data = JSON.parse(ev.data) as T
        this.conn.report(this.topic, 'ok')
        this.emit(data)
      } catch { /* ignore parse errors */ }
    }
    ws.onclose = () => {
      this.ws = null
      if (this.closed) return
      this.conn.report(this.topic, 'error')
      // Reopen with fixed 2s retry (matches lib/ws.ts convention). Exponential
      // backoff lives in BridgeSubscription per spec §5.5.
      if (this.subscribers.size > 0) {
        setTimeout(() => this.connect(), 2000)
      }
    }
    ws.onerror = () => ws.close()
  }

  protected disconnect(): void {
    this.closed = true
    this.ws?.close()
    this.ws = null
    this.conn.clear(this.topic)
  }
}
