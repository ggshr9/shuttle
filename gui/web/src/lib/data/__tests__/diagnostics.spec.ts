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

describe('Diagnostics — fallback persistence', () => {
  it('recordFallback writes to localStorage and updates snapshot', () => {
    const s = makeStorage()
    const d = new Diagnostics(s)
    d.recordFallback('timeout')
    const snap = d.snapshot()
    expect(snap.fallbacks).toHaveLength(1)
    expect(snap.fallbacks[0].reason).toBe('timeout')
    expect(snap.fallbacksTotal).toBe(1)
    const stored = JSON.parse(s.getItem('shuttle.diag.fallbacks')!)
    expect(stored.entries).toHaveLength(1)
    expect(stored.total).toBe(1)
  })

  it('caps fallbacks list at MAX (10) but keeps total monotonic', () => {
    const s = makeStorage()
    const d = new Diagnostics(s)
    for (let i = 0; i < 15; i++) d.recordFallback(`r${i}`)
    const snap = d.snapshot()
    expect(snap.fallbacks).toHaveLength(10)
    expect(snap.fallbacks[0].reason).toBe('r5')   // oldest kept
    expect(snap.fallbacks[9].reason).toBe('r14')  // newest
    expect(snap.fallbacksTotal).toBe(15)
  })

  it('hydrates from valid localStorage on construction', () => {
    const s = makeStorage()
    s.setItem('shuttle.diag.fallbacks', JSON.stringify({
      entries: [{ reason: 'old', at: 1000 }],
      total: 7,
    }))
    const d = new Diagnostics(s)
    const snap = d.snapshot()
    expect(snap.fallbacks).toEqual([{ reason: 'old', at: 1000 }])
    expect(snap.fallbacksTotal).toBe(7)
  })

  it('survives corrupt JSON gracefully (treats as empty)', () => {
    const s = makeStorage()
    s.setItem('shuttle.diag.fallbacks', 'not json {')
    const d = new Diagnostics(s)
    const snap = d.snapshot()
    expect(snap.fallbacks).toEqual([])
    expect(snap.fallbacksTotal).toBe(0)
  })

  it('loads total even when entries key is absent', () => {
    const s = makeStorage()
    s.setItem('shuttle.diag.fallbacks', JSON.stringify({ total: 5 }))
    const d = new Diagnostics(s)
    const snap = d.snapshot()
    expect(snap.fallbacks).toEqual([])
    expect(snap.fallbacksTotal).toBe(5)
  })

  it('drops malformed entries during hydrate', () => {
    const s = makeStorage()
    s.setItem('shuttle.diag.fallbacks', JSON.stringify({
      entries: [
        { reason: 'good', at: 100 },
        { reason: 123 },                        // bad type
        { at: 200 },                            // missing reason
        null,                                   // null
        { reason: 'also-good', at: 200 },
      ],
      total: 5,
    }))
    const d = new Diagnostics(s)
    expect(d.snapshot().fallbacks).toEqual([
      { reason: 'good', at: 100 },
      { reason: 'also-good', at: 200 },
    ])
  })

  it('swallows setItem errors (storage quota / disabled)', () => {
    const s: Storage = {
      ...makeStorage(),
      setItem: () => { throw new Error('QuotaExceeded') },
    }
    const d = new Diagnostics(s)
    expect(() => d.recordFallback('boom')).not.toThrow()
    // in-memory state still updated
    expect(d.snapshot().fallbacksTotal).toBe(1)
  })
})

describe('Diagnostics — persistDirect (no instance)', () => {
  it('writes a fallback entry without an instance', () => {
    const s = makeStorage()
    Diagnostics.persistDirect('early-fail', s)
    const stored = JSON.parse(s.getItem('shuttle.diag.fallbacks')!)
    expect(stored.entries).toHaveLength(1)
    expect(stored.entries[0].reason).toBe('early-fail')
    expect(stored.total).toBe(1)
  })

  it('appends to existing storage and a later instance hydrates correctly', () => {
    const s = makeStorage()
    Diagnostics.persistDirect('first', s)
    Diagnostics.persistDirect('second', s)
    const d = new Diagnostics(s)
    const snap = d.snapshot()
    expect(snap.fallbacksTotal).toBe(2)
    expect(snap.fallbacks.map(e => e.reason)).toEqual(['first', 'second'])
  })

  it('persistDirect respects MAX_FALLBACKS cap', () => {
    const s = makeStorage()
    for (let i = 0; i < 15; i++) Diagnostics.persistDirect(`r${i}`, s)
    const stored = JSON.parse(s.getItem('shuttle.diag.fallbacks')!)
    expect(stored.entries).toHaveLength(10)
    expect(stored.total).toBe(15)
  })

  it('persistDirect swallows setItem errors', () => {
    const s: Storage = { ...makeStorage(), setItem: () => { throw new Error('full') } }
    expect(() => Diagnostics.persistDirect('boom', s)).not.toThrow()
  })
})

describe('Diagnostics — reset', () => {
  it('clears in-memory + localStorage', () => {
    const s = makeStorage()
    const d = new Diagnostics(s)
    d.recordRequest(20, false, 'err')
    d.recordFallback('boom')
    d.reset()
    const snap = d.snapshot()
    expect(snap.requestsTotal).toBe(0)
    expect(snap.requestsErr).toBe(0)
    expect(snap.lastError).toBeNull()
    expect(snap.fallbacks).toEqual([])
    expect(snap.fallbacksTotal).toBe(0)
    expect(s.getItem('shuttle.diag.fallbacks')).toBeNull()
  })
})
