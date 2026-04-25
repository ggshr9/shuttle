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

describe('Diagnostics — RTT samples', () => {
  it('returns null p50/p95 with fewer than 10 samples', () => {
    const d = new Diagnostics(makeStorage())
    for (let i = 0; i < 9; i++) d.recordRequest(10, true)
    const s = d.snapshot()
    expect(s.rttP50).toBeNull()
    expect(s.rttP95).toBeNull()
  })

  it('returns sorted percentiles at exactly 10 samples', () => {
    const d = new Diagnostics(makeStorage())
    // values 1..10 → p50 ≈ 5 or 6, p95 ≈ 10
    for (let v = 1; v <= 10; v++) d.recordRequest(v, true)
    const s = d.snapshot()
    expect(s.rttP50).toBeGreaterThanOrEqual(5)
    expect(s.rttP50).toBeLessThanOrEqual(6)
    expect(s.rttP95).toBe(10)
  })

  it('handles odd-sized window correctly', () => {
    const d = new Diagnostics(makeStorage())
    const vs = [10, 30, 20, 50, 40, 70, 60, 90, 80, 100, 110]   // 11 values
    for (const v of vs) d.recordRequest(v, true)
    const s = d.snapshot()
    // sorted: 10,20,30,40,50,60,70,80,90,100,110 → p50 = index 5 = 60
    expect(s.rttP50).toBe(60)
  })

  it('drops oldest sample when ring buffer exceeds 100', () => {
    const d = new Diagnostics(makeStorage())
    // First 100 are 1, next 100 are 1000 — after pushing 200, only 1000s remain
    for (let i = 0; i < 100; i++) d.recordRequest(1, true)
    for (let i = 0; i < 100; i++) d.recordRequest(1000, true)
    const s = d.snapshot()
    expect(s.rttP50).toBe(1000)
    expect(s.rttP95).toBe(1000)
  })

  it('includes failed-request duration in rtt samples', () => {
    const d = new Diagnostics(makeStorage())
    for (let i = 0; i < 9; i++) d.recordRequest(10, true)
    d.recordRequest(500, false, 'err')
    const s = d.snapshot()
    expect(s.rttP50).not.toBeNull()
    expect(s.rttP95).toBe(500)
  })
})
