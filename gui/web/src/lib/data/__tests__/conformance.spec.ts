// gui/web/src/lib/data/__tests__/conformance.spec.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { HttpAdapter } from '../http-adapter'
import { BridgeAdapter } from '../bridge-adapter'
import { ApiError, TransportError, type DataAdapter } from '../types'

type AdapterFactory = () => Promise<DataAdapter> | DataAdapter

class FakeWS {
  static instances: FakeWS[] = []
  static reset() { FakeWS.instances = [] }
  url: string
  readyState = 0
  onopen?: () => void
  onmessage?: (e: { data: string }) => void
  onclose?: () => void
  onerror?: () => void
  closed = false
  constructor(url: string) {
    this.url = url
    FakeWS.instances.push(this)
    queueMicrotask(() => { this.readyState = 1; this.onopen?.() })
  }
  send(_: string) {}
  close() { this.closed = true; this.readyState = 3; this.onclose?.() }
  push(payload: unknown) { this.onmessage?.({ data: JSON.stringify(payload) }) }
}

const factories: Array<[string, AdapterFactory]> = [
  ['http', () => new HttpAdapter()],
  ['bridge', () => {
    // Wire window.ShuttleBridge to the test's fetch mock so that request-tier
    // tests can assert behavior identically across both transports.
    ;(globalThis as any).window = {
      ShuttleBridge: {
        send: async (env: any) => {
          const fetchMock = (globalThis as any).fetch
          if (!fetchMock) throw new Error('no fetch mock — bridge needs request stub')
          const init: RequestInit = {
            method: env.method,
            headers: env.headers,
            body: env.body ? atob(env.body) : undefined,
          }
          let res: Response
          try {
            res = await fetchMock(env.path, init)
          } catch (err) {
            // Network-level failure — surface as transport error in envelope.
            return { status: -1, headers: {}, body: '', error: String(err) }
          }
          const headers: Record<string, string> = {}
          res.headers?.forEach((v, k) => { headers[k] = v })
          const text = res.status === 204 ? '' : await res.text().catch(() => '')
          return {
            status: res.status,
            headers,
            body: btoa(text),
            error: null,
          }
        },
      },
    }
    return new BridgeAdapter()
  }],
]

type AdapterName = 'http' | 'bridge'

/**
 * drivesValue triggers a snapshot emission via whichever mechanism the adapter
 * uses, so tests don't have to branch on adapter name.
 *
 * `setup` is called to create the subscription and register callbacks. For
 * bridge, setup runs INSIDE this function after fake timers are installed so
 * that connect()'s setTimeout(0) lands in the fake-timer queue and is
 * controlled by runOnlyPendingTimersAsync.
 *
 * Pass `setup` only on the FIRST call. Omit it on subsequent calls (dedup /
 * unsubscribe tests) — the subscription already exists and the next poll timer
 * is already scheduled; this function just advances time and pushes again.
 */
async function drivesValue(
  name: AdapterName,
  value: unknown,
  setup?: () => void,
): Promise<void> {
  if (name === 'bridge') {
    ;(globalThis as any).fetch = vi.fn(async () =>
      new Response(JSON.stringify(value), { status: 200, headers: { 'content-type': 'application/json' } }))
    vi.useFakeTimers()
    // On first call: setup() registers the subscription under fake timers.
    // On subsequent calls: the next-poll setTimeout is already in the fake
    // queue (survives vi.useRealTimers / vi.useFakeTimers round-trip).
    setup?.()
    await vi.runOnlyPendingTimersAsync()
    vi.useRealTimers()
  } else {
    // For HTTP, setup() creates the subscription and its FakeWS; the WS
    // queues onopen via microtask. Wait for that to fire, then push.
    setup?.()
    await Promise.resolve()
    FakeWS.instances[0].push(value)
  }
}

describe.each(factories)('%s adapter conformance', (_name, factory) => {
  let adapter: DataAdapter

  beforeEach(async () => {
    vi.useRealTimers()        // reset any leaked fake-timer state from a prior test failure
    FakeWS.reset()
    ;(globalThis as any).WebSocket = FakeWS
    adapter = await factory()
  })

  describe('request', () => {
    it('parses 200 JSON', async () => {
      ;(globalThis as any).fetch = vi.fn(async () =>
        new Response(JSON.stringify({ ok: true }), { status: 200, headers: { 'content-type': 'application/json' } }))
      expect(await adapter.request({ method: 'GET', path: '/api/x' })).toEqual({ ok: true })
    })

    it('returns undefined for 204', async () => {
      ;(globalThis as any).fetch = vi.fn(async () => new Response(null, { status: 204 }))
      expect(await adapter.request({ method: 'GET', path: '/api/x' })).toBeUndefined()
    })

    it('throws ApiError on 4xx', async () => {
      ;(globalThis as any).fetch = vi.fn(async () =>
        new Response(JSON.stringify({ error: 'bad' }), { status: 400, headers: { 'content-type': 'application/json' } }))
      await expect(adapter.request({ method: 'GET', path: '/x' })).rejects.toBeInstanceOf(ApiError)
    })

    it('throws TransportError on network failure', async () => {
      ;(globalThis as any).fetch = vi.fn(async () => { throw new TypeError('boom') })
      await expect(adapter.request({ method: 'GET', path: '/x' })).rejects.toBeInstanceOf(TransportError)
    })

    it('honors AbortSignal', async () => {
      ;(globalThis as any).fetch = vi.fn(async (_: any, init: any) => {
        return new Promise<Response>((_resolve, reject) => {
          init.signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')))
        })
      })
      const ctl = new AbortController()
      const p = adapter.request({ method: 'GET', path: '/x', signal: ctl.signal })
      ctl.abort()
      await expect(p).rejects.toBeDefined()
    })
  })

  describe('subscribe (snapshot)', () => {
    it('emits values to subscribers', async () => {
      const cb = vi.fn()
      await drivesValue(_name as AdapterName, { connected: true }, () => {
        const sub = adapter.subscribe('status')
        sub.subscribe(cb)
      })
      expect(cb).toHaveBeenCalledWith(expect.objectContaining({ connected: true }))
    })

    it('does not emit when value unchanged', async () => {
      const cb = vi.fn()
      // First emission
      await drivesValue(_name as AdapterName, { connected: true }, () => {
        const sub = adapter.subscribe('status')
        sub.subscribe(cb)
      })
      // Second emission — same value, should be deduped
      await drivesValue(_name as AdapterName, { connected: true })
      expect(cb).toHaveBeenCalledTimes(1)
    })

    it('current() returns last value', async () => {
      let sub: ReturnType<typeof adapter.subscribe>
      await drivesValue(_name as AdapterName, { connected: true }, () => {
        sub = adapter.subscribe('status')
        sub.subscribe(() => {})
      })
      expect(sub!.current).toEqual({ connected: true })
    })

    it('multiple subscribers all receive updates', async () => {
      const a = vi.fn(); const b = vi.fn()
      await drivesValue(_name as AdapterName, { connected: true }, () => {
        const sub = adapter.subscribe('status')
        sub.subscribe(a); sub.subscribe(b)
      })
      expect(a).toHaveBeenCalled(); expect(b).toHaveBeenCalled()
    })

    it('unsubscribe stops emissions', async () => {
      let off: () => void
      const cb = vi.fn()
      // First emission — confirm subscription is working
      await drivesValue(_name as AdapterName, { connected: true }, () => {
        const sub = adapter.subscribe('status')
        off = sub.subscribe(cb)
      })
      expect(cb).toHaveBeenCalledTimes(1)
      off!()
      // Attempt a second emission — cb should not be called again
      await drivesValue(_name as AdapterName, { connected: true })
      expect(cb).toHaveBeenCalledTimes(1)
    })
  })

  describe('subscribe (stream)', () => {
    it('does not replay history to new subscribers', async () => {
      if (_name === 'bridge') {
        ;(globalThis as any).fetch = vi.fn(async () =>
          new Response(
            JSON.stringify({ lines: [{ timestamp: '1', level: 'info', message: 'hello' }], cursor: 1 }),
            { status: 200, headers: { 'content-type': 'application/json' } },
          ))
        vi.useFakeTimers()
        const sub = adapter.subscribe('logs')
        const a = vi.fn()
        sub.subscribe(a)
        // Poll once — a sees the log line
        await vi.runOnlyPendingTimersAsync()
        expect(a).toHaveBeenCalledTimes(1)
        // Second subscriber added after first poll; should NOT receive old event
        const b = vi.fn()
        sub.subscribe(b)
        expect(b).not.toHaveBeenCalled()
        vi.useRealTimers()
        return
      }
      // HTTP path
      const sub = adapter.subscribe('logs')
      const a = vi.fn()
      sub.subscribe(a)
      await Promise.resolve()
      FakeWS.instances[0].push({ ts: '1', level: 'info', msg: 'hello' })
      const b = vi.fn()
      sub.subscribe(b)
      expect(b).not.toHaveBeenCalled()
    })

    it('current() is undefined for stream topics', () => {
      const sub = adapter.subscribe('logs')
      sub.subscribe(() => {})
      expect(sub.current).toBeUndefined()
    })
  })

  describe('connectionState', () => {
    it('starts idle', () => {
      expect(adapter.connectionState.value).toBe('idle')
    })

    it('reaches connected after first message', async () => {
      if (_name === 'bridge') {
        ;(globalThis as any).fetch = vi.fn(async () =>
          new Response(JSON.stringify({ connected: true }), { status: 200, headers: { 'content-type': 'application/json' } }))
        vi.useFakeTimers()
        const sub = adapter.subscribe('status')
        sub.subscribe(() => {})
        await vi.runOnlyPendingTimersAsync()
        expect(adapter.connectionState.value).toBe('connected')
        vi.useRealTimers()
        return
      }
      // HTTP path
      const sub = adapter.subscribe('status')
      sub.subscribe(() => {})
      await Promise.resolve()
      FakeWS.instances[0].push({ connected: true })
      expect(adapter.connectionState.value).toBe('connected')
    })
  })
})

describe.each(factories)('%s adapter — diagnostics conformance', (_label, factory) => {
  it('reports identical counts after the same workload', async () => {
    const a = await factory()
    let callCount = 0
    ;(globalThis as any).fetch = vi.fn(async () => {
      callCount++
      if (callCount === 1) return new Response('{}', { status: 200 })
      if (callCount === 2) return new Response(JSON.stringify({ error: 'x' }), { status: 500 })
      throw new TypeError('network down')
    })
    await a.request({ method: 'GET', path: '/a' }).catch(() => {})
    await a.request({ method: 'GET', path: '/b' }).catch(() => {})
    await a.request({ method: 'GET', path: '/c' }).catch(() => {})
    const snap = a.diagnostics.snapshot()
    expect(snap.requestsTotal).toBe(3)
    expect(snap.requestsErr).toBe(2)
    expect(snap.errorRate).toBeCloseTo(2 / 3, 3)
  })
})
