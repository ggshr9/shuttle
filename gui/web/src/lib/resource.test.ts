import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import {
  createResource,
  createStream,
  invalidate,
  __resetRegistry,
  __resetStreams,
} from '@/lib/resource.svelte'

beforeEach(() => {
  __resetRegistry()
  __resetStreams()
  vi.useRealTimers()
})

afterEach(() => {
  __resetStreams()
})

describe('createResource', () => {
  it('fetches on first subscribe and populates data', async () => {
    const fetcher = vi.fn(async () => ({ v: 1 }))
    const r = createResource('test.a', fetcher)
    expect(r.loading).toBe(true)
    await vi.waitUntil(() => !r.loading, { timeout: 500 })
    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(r.data).toEqual({ v: 1 })
    expect(r.error).toBeNull()
  })

  it('shares one fetcher across same key', async () => {
    const fetcher = vi.fn(async () => ({ v: 2 }))
    const a = createResource('test.shared', fetcher)
    const b = createResource('test.shared', fetcher)
    await vi.waitUntil(() => !a.loading, { timeout: 500 })
    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(a.data).toBe(b.data)
  })

  it('preserves last data when fetch fails', async () => {
    let call = 0
    const fetcher = vi.fn(async () => {
      call++
      if (call === 2) throw new Error('boom')
      return { v: call }
    })
    const r = createResource('test.err', fetcher)
    await vi.waitUntil(() => !r.loading, { timeout: 500 })
    expect(r.data).toEqual({ v: 1 })
    await r.refetch()
    expect(r.data).toEqual({ v: 1 })     // preserved
    expect(r.error?.message).toBe('boom')
    expect(r.stale).toBe(true)
  })

  it('invalidate triggers refetch on subscribed key', async () => {
    let counter = 0
    const fetcher = vi.fn(async () => ({ v: ++counter }))
    const r = createResource('test.invalidate', fetcher)
    await vi.waitUntil(() => !r.loading, { timeout: 500 })
    const first = r.data
    invalidate('test.invalidate')
    await vi.waitUntil(() => r.data !== first, { timeout: 500 })
    expect(fetcher).toHaveBeenCalledTimes(2)
  })
})

describe('createStream', () => {
  it('opens one WebSocket per key regardless of subscriber count', () => {
    const spy = vi.fn()
    // Count WebSocket constructor invocations by replacing the global.
    const origWS = globalThis.WebSocket
    globalThis.WebSocket = class {
      onmessage: ((e: MessageEvent) => void) | null = null
      onclose: (() => void) | null = null
      onerror: (() => void) | null = null
      constructor(..._args: unknown[]) { spy() }
      close() {}
    } as unknown as typeof WebSocket

    try {
      const a = createStream('stream.dedup', '/api/test')
      const b = createStream('stream.dedup', '/api/test')
      const c = createStream('stream.dedup', '/api/test')
      expect(spy).toHaveBeenCalledTimes(1)
      // All three subscribers see the same backing state.
      expect(a.data).toBe(b.data)
      expect(b.data).toBe(c.data)
    } finally {
      globalThis.WebSocket = origWS
    }
  })

  it('closes the socket only when last subscriber closes', () => {
    let closeCalls = 0
    const origWS = globalThis.WebSocket
    globalThis.WebSocket = class {
      onmessage: ((e: MessageEvent) => void) | null = null
      onclose: (() => void) | null = null
      onerror: (() => void) | null = null
      constructor(..._args: unknown[]) {}
      close() { closeCalls++ }
    } as unknown as typeof WebSocket

    try {
      const a = createStream('stream.refcount', '/api/test')
      const b = createStream('stream.refcount', '/api/test')
      a.close()
      expect(closeCalls).toBe(0)     // b still subscribed
      b.close()
      expect(closeCalls).toBe(1)     // both gone → close
    } finally {
      globalThis.WebSocket = origWS
    }
  })
})
