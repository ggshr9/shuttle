import { describe, it, expect } from 'vitest'
import { Diagnostics } from '../diagnostics.svelte'

function makeStorage(): Storage {
  const m = new Map<string, string>()
  return {
    get length() { return m.size },
    clear() { m.clear() },
    getItem(k) { return m.get(k) ?? null },
    setItem(k, v) { m.set(k, v) },
    removeItem(k) { m.delete(k) },
    key(i) { return [...m.keys()][i] ?? null },
  }
}

describe('Diagnostics — request counters', () => {
  it('starts with zero counts and null lastError', () => {
    const d = new Diagnostics(makeStorage())
    const s = d.snapshot()
    expect(s.requestsTotal).toBe(0)
    expect(s.requestsErr).toBe(0)
    expect(s.errorRate).toBe(0)
    expect(s.lastError).toBeNull()
  })

  it('recordRequest(ok=true) increments only requestsTotal', () => {
    const d = new Diagnostics(makeStorage())
    d.recordRequest(15, true)
    d.recordRequest(20, true)
    const s = d.snapshot()
    expect(s.requestsTotal).toBe(2)
    expect(s.requestsErr).toBe(0)
    expect(s.errorRate).toBe(0)
    expect(s.lastError).toBeNull()
  })

  it('recordRequest(ok=false) increments errors and sets lastError', () => {
    const d = new Diagnostics(makeStorage())
    const t0 = Date.now()
    d.recordRequest(50, false, 'TransportError: timeout')
    const s = d.snapshot()
    expect(s.requestsTotal).toBe(1)
    expect(s.requestsErr).toBe(1)
    expect(s.errorRate).toBeCloseTo(1.0, 5)
    expect(s.lastError).not.toBeNull()
    expect(s.lastError!.reason).toBe('TransportError: timeout')
    expect(s.lastError!.at).toBeGreaterThanOrEqual(t0)
  })

  it('errorRate computed correctly across mixed', () => {
    const d = new Diagnostics(makeStorage())
    for (let i = 0; i < 9; i++) d.recordRequest(10, true)
    d.recordRequest(10, false, 'oops')
    const s = d.snapshot()
    expect(s.requestsTotal).toBe(10)
    expect(s.requestsErr).toBe(1)
    expect(s.errorRate).toBeCloseTo(0.1, 5)
  })

  it('recordRequest with no reason defaults to "unknown" in lastError', () => {
    const d = new Diagnostics(makeStorage())
    d.recordRequest(10, false)
    expect(d.snapshot().lastError!.reason).toBe('unknown')
  })
})
