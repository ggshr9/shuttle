import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { BridgeSubscription } from '../bridge-subscription'
import { ConnectionStateController } from '../connection-state'

function makeFetcher(impl: (path: string) => Promise<any>) {
  return vi.fn(impl)
}

describe('BridgeSubscription (snapshot)', () => {
  let conn: ConnectionStateController
  beforeEach(() => {
    vi.useFakeTimers()
    conn = new ConnectionStateController()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('polls immediately on first subscribe', async () => {
    const fetcher = makeFetcher(async () => ({ a: 1 }))
    const sub = new BridgeSubscription<{ a: number }>(
      'status', 'snapshot', '/api/status', 2000, undefined, fetcher, conn,
    )
    sub.add(() => {})
    await vi.runOnlyPendingTimersAsync()
    expect(fetcher).toHaveBeenCalledTimes(1)
  })

  it('emits diff only — same value does not re-emit', async () => {
    const fetcher = makeFetcher(async () => ({ a: 1 }))
    const sub = new BridgeSubscription<{ a: number }>(
      'status', 'snapshot', '/api/status', 100, undefined, fetcher, conn,
    )
    const cb = vi.fn()
    sub.add(cb)
    await vi.advanceTimersByTimeAsync(0)
    await vi.advanceTimersByTimeAsync(120)
    expect(cb).toHaveBeenCalledTimes(1)
  })

  it('inFlight prevents pile-up', async () => {
    let resolve!: (v: any) => void
    const fetcher = makeFetcher(() => new Promise(r => { resolve = r }))
    const sub = new BridgeSubscription<any>(
      'status', 'snapshot', '/api/status', 50, undefined, fetcher, conn,
    )
    sub.add(() => {})
    await vi.advanceTimersByTimeAsync(150)
    expect(fetcher).toHaveBeenCalledTimes(1)  // first still pending
    resolve({ x: 1 })
  })
})

describe('BridgeSubscription (stream)', () => {
  let conn: ConnectionStateController
  beforeEach(() => { vi.useFakeTimers(); conn = new ConnectionStateController() })
  afterEach(() => { vi.useRealTimers() })

  it('passes cursor as ?since=N', async () => {
    const calls: string[] = []
    const fetcher = makeFetcher(async (p) => {
      calls.push(p)
      return { lines: [{ ts: '1', level: 'info', msg: 'hi' }], cursor: 5 }
    })
    const sub = new BridgeSubscription<any>(
      'logs', 'stream', '/api/logs', 50, 'since', fetcher, conn,
    )
    sub.add(() => {})
    await vi.advanceTimersByTimeAsync(0)
    expect(calls[0]).toContain('?since=0')
    await vi.advanceTimersByTimeAsync(60)
    // Second tick uses updated cursor
    expect(calls[1]).toContain('?since=5')
  })

  it('emits each line', async () => {
    const fetcher = makeFetcher(async () => ({ lines: [{ msg: 'a' }, { msg: 'b' }], cursor: 2 }))
    const sub = new BridgeSubscription<any>(
      'logs', 'stream', '/api/logs', 100, 'since', fetcher, conn,
    )
    const cb = vi.fn()
    sub.add(cb)
    await vi.advanceTimersByTimeAsync(0)
    expect(cb).toHaveBeenCalledTimes(2)
  })
})
