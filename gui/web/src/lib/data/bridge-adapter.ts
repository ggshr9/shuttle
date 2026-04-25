// gui/web/src/lib/data/bridge-adapter.ts
import { BridgeTransport } from './bridge-transport'
import { BridgeSubscription } from './bridge-subscription'
import { ConnectionStateController } from './connection-state'
import { topicConfig, type TopicKey, type TopicValue } from './topics'
import {
  ApiError, TransportError,
  type DataAdapter, type RequestOptions, type SubscribeOptions, type Subscription,
} from './types'
import { safeJson } from './internal/json'
import { Diagnostics } from './diagnostics.svelte'

export interface BridgeAdapterOptions {
  authToken?: () => string
  transport?: BridgeTransport
}

export class BridgeAdapter implements DataAdapter {
  readonly connectionState = new ConnectionStateController()
  readonly diagnostics = new Diagnostics()
  private readonly subs = new Map<TopicKey, BridgeSubscription<any>>()
  private readonly transport: BridgeTransport
  private readonly authToken: () => string

  constructor(opts: BridgeAdapterOptions = {}) {
    this.transport = opts.transport ?? new BridgeTransport()
    this.authToken = opts.authToken ?? (() => (typeof window !== 'undefined' ? (window as any).__SHUTTLE_AUTH_TOKEN__ ?? '' : ''))
  }

  async request<T = unknown>(opts: RequestOptions): Promise<T> {
    const t0 = (typeof performance !== 'undefined' ? performance : Date).now()
    let ok = false
    let reason: string | undefined
    try {
      const result = await this.#requestImpl<T>(opts)
      ok = true
      return result
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        // user-initiated abort — count as request, not error
        ok = true
        throw err
      }
      reason = err instanceof Error ? err.message : String(err)
      throw err
    } finally {
      const dt = (typeof performance !== 'undefined' ? performance : Date).now() - t0
      this.diagnostics.recordRequest(dt, ok, reason)
    }
  }

  async #requestImpl<T = unknown>(opts: RequestOptions): Promise<T> {
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
    let abortCleanup = () => {}
    try {
      if (opts.signal) {
        // The native envelope can't be cancelled mid-flight, but we honor the
        // contract by rejecting locally as soon as the signal aborts. The
        // envelope completes natively and its response is discarded.
        resp = await Promise.race([
          this.transport.send(envelope),
          new Promise<never>((_, reject) => {
            const handler = () => reject(new DOMException('Aborted', 'AbortError'))
            opts.signal!.addEventListener('abort', handler, { once: true })
            abortCleanup = () => opts.signal!.removeEventListener('abort', handler)
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
    } finally {
      abortCleanup()
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
