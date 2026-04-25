// gui/web/src/lib/data/bridge-adapter.ts
import { BridgeTransport } from './bridge-transport'
import { BridgeSubscription } from './bridge-subscription'
import { ConnectionStateController } from './connection-state'
import { topicConfig, type TopicKey, type TopicValue } from './topics'
import {
  ApiError, TransportError,
  type DataAdapter, type RequestOptions, type SubscribeOptions, type Subscription,
} from './types'

export interface BridgeAdapterOptions {
  authToken?: () => string
  transport?: BridgeTransport
}

export class BridgeAdapter implements DataAdapter {
  readonly connectionState = new ConnectionStateController()
  private readonly subs = new Map<TopicKey, BridgeSubscription<any>>()
  private readonly transport: BridgeTransport
  private readonly authToken: () => string

  constructor(opts: BridgeAdapterOptions = {}) {
    this.transport = opts.transport ?? new BridgeTransport()
    this.authToken = opts.authToken ?? (() => (typeof window !== 'undefined' ? (window as any).__SHUTTLE_AUTH_TOKEN__ ?? '' : ''))
  }

  async request<T = unknown>(opts: RequestOptions): Promise<T> {
    const token = this.authToken()
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...(opts.headers ?? {}),
    }
    if (token && !headers['Authorization']) headers['Authorization'] = `Bearer ${token}`

    const envelope = {
      method: opts.method,
      path: opts.path,
      headers,
      body: opts.body !== undefined ? btoa(JSON.stringify(opts.body)) : undefined,
    }

    // Pre-check for already-aborted signal — short-circuit before envelope IPC.
    if (opts.signal?.aborted) {
      throw new DOMException('Aborted', 'AbortError')
    }

    let resp
    try {
      if (opts.signal) {
        // The native envelope can't be cancelled mid-flight, but we honor the
        // contract by rejecting locally as soon as the signal aborts. The
        // envelope completes natively and its response is discarded.
        resp = await Promise.race([
          this.transport.send(envelope),
          new Promise<never>((_, reject) => {
            opts.signal!.addEventListener(
              'abort',
              () => reject(new DOMException('Aborted', 'AbortError')),
              { once: true },
            )
          }),
        ])
      } else {
        resp = await this.transport.send(envelope)
      }
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        throw err   // Don't wrap as TransportError
      }
      throw new TransportError(err, err instanceof Error ? err.message : String(err))
    }

    if (resp.status === -1 || resp.error) {
      throw new TransportError(null, resp.error || 'transport error')
    }

    if (resp.status === 204) return undefined as T

    const text = resp.body ? atob(resp.body) : ''
    const parsed = text ? safeJson(text) : undefined

    if (resp.status >= 400) {
      const msg = (parsed && typeof parsed === 'object' && 'error' in parsed) ? String((parsed as any).error) : `HTTP ${resp.status}`
      const code = (parsed && typeof parsed === 'object' && 'code' in parsed) ? String((parsed as any).code) : undefined
      throw new ApiError(resp.status, code, msg)
    }
    return parsed as T
  }

  subscribe<K extends TopicKey>(topic: K, _opts?: SubscribeOptions<K>): Subscription<TopicValue<K>> {
    let sub = this.subs.get(topic) as BridgeSubscription<TopicValue<K>> | undefined
    if (!sub) {
      const cfg = topicConfig[topic]
      const fetcher = async (path: string) => this.request({ method: 'GET', path })
      sub = new BridgeSubscription<TopicValue<K>>(
        topic, cfg.kind, cfg.restPath, cfg.pollMs, cfg.cursorParam, fetcher, this.connectionState,
      )
      this.subs.set(topic, sub)
    }
    return {
      get current() { return sub!.current },
      subscribe: cb => sub!.add(cb),
    }
  }
}

// Parses JSON, falling back to the raw string on parse failure. On the
// success (2xx) path callers consume the return as T; a raw-string return
// would indicate a server contract violation. On the error (4xx/5xx) path
// the ApiError extraction guards against non-object parsed shapes.
function safeJson(s: string): unknown {
  try { return JSON.parse(s) } catch { return s }
}
