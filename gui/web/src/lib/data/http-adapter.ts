// gui/web/src/lib/data/http-adapter.ts
import { ConnectionStateController } from './connection-state'
import { Diagnostics } from './diagnostics.svelte'
import { HttpSubscription } from './http-subscription'
import { topicConfig, type TopicKey, type TopicValue } from './topics'
import {
  ApiError, TransportError,
  type DataAdapter, type RequestOptions, type SubscribeOptions, type Subscription,
} from './types'
import { safeJson } from './internal/json'

export interface HttpAdapterOptions {
  base?: string                    // URL base, default ''
  authToken?: () => string         // pulled per-request
  defaultTimeoutMs?: number        // default 10_000
}

export class HttpAdapter implements DataAdapter {
  readonly connectionState = new ConnectionStateController()
  readonly diagnostics = new Diagnostics()
  private readonly subs = new Map<TopicKey, HttpSubscription<any>>()
  private readonly base: string
  private readonly authToken: () => string
  private readonly defaultTimeoutMs: number

  constructor(opts: HttpAdapterOptions = {}) {
    this.base = opts.base ?? ''
    this.authToken = opts.authToken ?? (() => (typeof window !== 'undefined' ? (window as any).__SHUTTLE_AUTH_TOKEN__ ?? '' : ''))
    this.defaultTimeoutMs = opts.defaultTimeoutMs ?? 10_000
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
    const { method, path, body, headers, signal, timeoutMs } = opts
    const ctrl = new AbortController()
    const linkResult = signal
      ? linkSignals(signal, ctrl.signal)
      : { signal: ctrl.signal, cleanup: () => {} }
    let timedOut = false
    const timer = setTimeout(() => { timedOut = true; ctrl.abort() }, timeoutMs ?? this.defaultTimeoutMs)
    try {
      const tok = this.authToken()
      const finalHeaders: Record<string, string> = {
        'Content-Type': 'application/json',
        ...(headers ?? {}),
      }
      if (tok && !finalHeaders['Authorization']) finalHeaders['Authorization'] = `Bearer ${tok}`
      const init: RequestInit = { method, headers: finalHeaders, signal: linkResult.signal }
      if (body !== undefined) init.body = JSON.stringify(body)

      let res: Response
      try {
        res = await fetch(this.base + path, init)
      } catch (err) {
        if (err instanceof DOMException && err.name === 'AbortError') {
          // Internal timeout → record as error with reason='timeout'.
          // External (caller-supplied) signal abort → pass through so outer
          // wrapper's carve-out treats it as ok (user cancellation).
          if (timedOut) throw new TransportError(err, 'timeout')
          throw err
        }
        throw new TransportError(err, err instanceof Error ? err.message : String(err))
      }

      if (res.status === 204) return undefined as T
      const text = await res.text().catch(() => '')
      const parsed = text ? safeJson(text) : undefined
      if (!res.ok) {
        const msg = (parsed && typeof parsed === 'object' && 'error' in parsed) ? String((parsed as any).error) : `HTTP ${res.status}`
        const code = (parsed && typeof parsed === 'object' && 'code' in parsed) ? String((parsed as any).code) : undefined
        throw new ApiError(res.status, code, msg)
      }
      return parsed as T
    } finally {
      clearTimeout(timer)
      linkResult.cleanup()
    }
  }

  subscribe<K extends TopicKey>(topic: K, _opts?: SubscribeOptions<K>): Subscription<TopicValue<K>> {
    // _opts (cursor / pollInterval) is wired in BridgeSubscription (Task 4.2).
    // HttpSubscription drives its own reconnect cadence and reads no cursor.
    let sub = this.subs.get(topic) as HttpSubscription<TopicValue<K>> | undefined
    if (!sub) {
      const cfg = topicConfig[topic]
      sub = new HttpSubscription<TopicValue<K>>(topic, cfg.kind, cfg.wsPath, this.connectionState, this.authToken)
      this.subs.set(topic, sub)
    }
    return {
      get current() { return sub!.current },
      subscribe: cb => sub!.add(cb),
    }
  }
}

function linkSignals(a: AbortSignal, b: AbortSignal): { signal: AbortSignal; cleanup: () => void } {
  const ctrl = new AbortController()
  if (a.aborted) {
    ctrl.abort(a.reason)
    return { signal: ctrl.signal, cleanup: () => {} }
  }
  if (b.aborted) {
    ctrl.abort(b.reason)
    return { signal: ctrl.signal, cleanup: () => {} }
  }
  const onA = () => ctrl.abort(a.reason)
  const onB = () => ctrl.abort(b.reason)
  a.addEventListener('abort', onA, { once: true })
  b.addEventListener('abort', onB, { once: true })
  return {
    signal: ctrl.signal,
    cleanup: () => {
      a.removeEventListener('abort', onA)
      b.removeEventListener('abort', onB)
    },
  }
}
