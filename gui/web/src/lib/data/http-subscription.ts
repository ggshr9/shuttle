// gui/web/src/lib/data/http-subscription.ts
import { nextBackoffMs } from '../backoff'
import { SubscriptionBase } from './subscription-base'
import type { TopicKey, TopicKind } from './topics'
import type { ConnectionStateController } from './connection-state'

export class HttpSubscription<T> extends SubscriptionBase<T> {
  private ws: WebSocket | null = null
  private closed = false
  private reconnectAttempt = 0

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
    ws.onopen = () => {
      this.reconnectAttempt = 0 // success resets the backoff window
    }
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
      // Capped exponential backoff with jitter — matches the reconnect
      // discipline used by BridgeSubscription so HTTP and bridge paths
      // don't behave differently under outage.
      if (this.subscribers.size > 0) {
        const delay = nextBackoffMs(this.reconnectAttempt)
        this.reconnectAttempt++
        setTimeout(() => this.connect(), delay)
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
