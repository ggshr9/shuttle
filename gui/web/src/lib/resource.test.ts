import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createResource, invalidate, __resetRegistry } from '@/lib/resource.svelte'

beforeEach(() => {
  __resetRegistry()
  vi.useRealTimers()
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
