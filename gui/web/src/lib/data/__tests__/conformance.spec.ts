// gui/web/src/lib/data/__tests__/conformance.spec.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { HttpAdapter } from '../http-adapter'
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
  // ['bridge', async () => makeBridgeAdapter()] — added in Task 4.4
]

describe.each(factories)('%s adapter conformance', (_name, factory) => {
  let adapter: DataAdapter

  beforeEach(async () => {
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
      const sub = adapter.subscribe('status')
      const cb = vi.fn()
      sub.subscribe(cb)
      await Promise.resolve()
      FakeWS.instances[0].push({ connected: true })
      expect(cb).toHaveBeenCalledWith(expect.objectContaining({ connected: true }))
    })

    it('does not emit when value unchanged', async () => {
      const sub = adapter.subscribe('status')
      const cb = vi.fn()
      sub.subscribe(cb)
      await Promise.resolve()
      FakeWS.instances[0].push({ connected: true })
      FakeWS.instances[0].push({ connected: true })
      expect(cb).toHaveBeenCalledTimes(1)
    })

    it('current() returns last value', async () => {
      const sub = adapter.subscribe('status')
      sub.subscribe(() => {})
      await Promise.resolve()
      FakeWS.instances[0].push({ connected: true })
      expect(sub.current).toEqual({ connected: true })
    })

    it('multiple subscribers all receive updates', async () => {
      const sub = adapter.subscribe('status')
      const a = vi.fn(); const b = vi.fn()
      sub.subscribe(a); sub.subscribe(b)
      await Promise.resolve()
      FakeWS.instances[0].push({ connected: true })
      expect(a).toHaveBeenCalled(); expect(b).toHaveBeenCalled()
    })

    it('unsubscribe stops emissions', async () => {
      const sub = adapter.subscribe('status')
      const cb = vi.fn()
      const off = sub.subscribe(cb)
      await Promise.resolve()
      off()
      FakeWS.instances[0].push({ connected: true })
      expect(cb).not.toHaveBeenCalled()
    })
  })

  describe('subscribe (stream)', () => {
    it('does not replay history to new subscribers', async () => {
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
      const sub = adapter.subscribe('status')
      sub.subscribe(() => {})
      await Promise.resolve()
      FakeWS.instances[0].push({ connected: true })
      expect(adapter.connectionState.value).toBe('connected')
    })
  })
})
