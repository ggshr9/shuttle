import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setAdapter } from '../index'
import { useRequest } from '../hooks.svelte'
import type { DataAdapter, Subscription } from '../types'
import type { TopicKey } from '../topics'

function fakeSubscription<T>(initialValue?: T): Subscription<T> & { push(v: T): void } {
  let cb: ((v: T) => void) | null = null
  return {
    current: initialValue,
    subscribe(c) { cb = c; return () => { cb = null } },
    push(v) { cb?.(v) },
  }
}

function fakeAdapter(opts: {
  subFor?: (k: TopicKey) => Subscription<any>,
  request?: ReturnType<typeof vi.fn>,
} = {}): DataAdapter {
  return {
    request: opts.request ?? vi.fn().mockResolvedValue({}),
    subscribe: ((k: TopicKey) => opts.subFor?.(k) ?? fakeSubscription()) as DataAdapter['subscribe'],
    connectionState: {
      value: 'idle',
      subscribe: () => () => {},
    },
  }
}

describe('hooks (component-less smoke)', () => {
  beforeEach(() => {
    setAdapter(fakeAdapter())
  })

  it('useRequest delegates to adapter.request', async () => {
    const reqMock = vi.fn().mockResolvedValue({ ok: true })
    setAdapter(fakeAdapter({ request: reqMock }))
    const result = await useRequest({ method: 'GET', path: '/x' })
    expect(reqMock).toHaveBeenCalledWith({ method: 'GET', path: '/x' })
    expect(result).toEqual({ ok: true })
  })
})
