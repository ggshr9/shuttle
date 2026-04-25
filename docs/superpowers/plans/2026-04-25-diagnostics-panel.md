# Diagnostics Panel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Settings → Diagnostics sub-page that surfaces bridge health (request count / error rate / RTT p50/p95), the most-recent error, and a cross-session fallback history — implemented once via `DataAdapter` so the same code & UI work on iOS bridge / iOS proxy / Android / Wails / Web.

**Architecture:** Extend the existing `DataAdapter` interface with a `diagnostics: Diagnostics` field. Both `HttpAdapter` and `BridgeAdapter` instantiate the same `Diagnostics` class and wrap their `request()` in a timing try/finally. RTT samples + counters live in memory (Svelte 5 `$state`); fallback history persists to localStorage so it survives the SPA being torn down by `FallbackHandler`. `boot.ts` records fallbacks synchronously **before** posting the message that destroys the SPA.

**Tech Stack:** TypeScript, Svelte 5 (runes via `.svelte.ts` files), Vitest, jsdom.

---

## File Structure

| File | Responsibility | Action |
|---|---|---|
| `gui/web/src/lib/data/diagnostics.svelte.ts` | `Diagnostics` class + `DiagnosticsSnapshot` type | Create |
| `gui/web/src/lib/data/__tests__/diagnostics.spec.ts` | Unit tests for `Diagnostics` | Create |
| `gui/web/src/lib/data/types.ts` | Add `diagnostics: Diagnostics` to `DataAdapter` interface | Modify |
| `gui/web/src/lib/data/http-adapter.ts` | Instantiate `Diagnostics`, add timing wrapper | Modify |
| `gui/web/src/lib/data/bridge-adapter.ts` | Instantiate `Diagnostics`, add timing wrapper | Modify |
| `gui/web/src/lib/data/index.ts` | Add `tryGetAdapter()` helper | Modify |
| `gui/web/src/lib/data/__tests__/http-adapter.spec.ts` | Add diagnostics-recording test cases | Modify |
| `gui/web/src/lib/data/__tests__/bridge-adapter.spec.ts` | Add diagnostics-recording test cases | Modify |
| `gui/web/src/lib/data/__tests__/conformance.spec.ts` | Add cross-adapter diagnostics consistency tests | Modify |
| `gui/web/src/app/boot.ts` | Wire `requestFallback` to record before posting | Modify |
| `gui/web/src/app/__tests__/boot.spec.ts` | Test the recording-before-post contract | Modify |
| `gui/web/src/locales/en.json` | Add `settings.diag.*` and `settings.nav.diag` keys | Modify |
| `gui/web/src/locales/zh-CN.json` | Same | Modify |
| `gui/web/src/features/settings/sub/Diagnostics.svelte` | UI component | Create |
| `gui/web/src/features/settings/sub/__tests__/Diagnostics.test.ts` | Component tests | Create |
| `gui/web/src/features/settings/nav.ts` | Add diagnostics sub-page entry | Modify |
| `gui/web/src/features/settings/SettingsPage.svelte` | Wire new sub-page into pageMap | Modify |

---

## Test Verification

All tests run via:
```bash
cd gui/web && npm run test
```

Single-file:
```bash
cd gui/web && npm run test -- <relative-path-to-spec>
```

Type-check:
```bash
cd gui/web && npm run check
```

**Do NOT run `go test` directly anywhere.** This plan touches only `gui/web/`, no Go code, but the project guard rule applies in general.

---

## Task 1: Diagnostics class — request counters (in-memory)

**Files:**
- Create: `gui/web/src/lib/data/diagnostics.svelte.ts`
- Create: `gui/web/src/lib/data/__tests__/diagnostics.spec.ts`

- [ ] **Step 1: Write failing tests**

Create `gui/web/src/lib/data/__tests__/diagnostics.spec.ts`:

```typescript
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd gui/web && npm run test -- src/lib/data/__tests__/diagnostics.spec.ts
```
Expected: FAIL with "Cannot find module '../diagnostics.svelte'"

- [ ] **Step 3: Write minimal implementation**

Create `gui/web/src/lib/data/diagnostics.svelte.ts`:

```typescript
// gui/web/src/lib/data/diagnostics.svelte.ts

export interface DiagnosticsSnapshot {
  requestsTotal: number
  requestsErr: number
  errorRate: number
  rttP50: number | null
  rttP95: number | null
  lastError: { reason: string; at: number } | null
  fallbacks: { reason: string; at: number }[]
  fallbacksTotal: number
}

export class Diagnostics {
  // in-memory reactive state
  #requestsTotal = $state(0)
  #requestsErr = $state(0)
  #lastError = $state<{ reason: string; at: number } | null>(null)

  constructor(_storage: Storage = globalThis.localStorage) {
    // storage usage lands in Task 3
  }

  recordRequest(_durationMs: number, ok: boolean, reason?: string): void {
    this.#requestsTotal++
    if (!ok) {
      this.#requestsErr++
      this.#lastError = { reason: reason ?? 'unknown', at: Date.now() }
    }
  }

  snapshot(): DiagnosticsSnapshot {
    const total = this.#requestsTotal
    return {
      requestsTotal: total,
      requestsErr: this.#requestsErr,
      errorRate: total > 0 ? this.#requestsErr / total : 0,
      rttP50: null,
      rttP95: null,
      lastError: this.#lastError,
      fallbacks: [],
      fallbacksTotal: 0,
    }
  }
}
```

- [ ] **Step 4: Run tests, expect pass**

```bash
cd gui/web && npm run test -- src/lib/data/__tests__/diagnostics.spec.ts
```
Expected: 5 PASSING

- [ ] **Step 5: Commit**

```bash
git add gui/web/src/lib/data/diagnostics.svelte.ts gui/web/src/lib/data/__tests__/diagnostics.spec.ts
git commit -m "feat(diagnostics): Diagnostics class — request counters in-memory"
```

---

## Task 2: Diagnostics — RTT ring buffer + p50/p95

**Files:**
- Modify: `gui/web/src/lib/data/diagnostics.svelte.ts`
- Modify: `gui/web/src/lib/data/__tests__/diagnostics.spec.ts`

- [ ] **Step 1: Add failing tests**

Append to `diagnostics.spec.ts`:

```typescript
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
})
```

- [ ] **Step 2: Run, expect 4 new failures (rttP50/P95 still null)**

```bash
cd gui/web && npm run test -- src/lib/data/__tests__/diagnostics.spec.ts
```

- [ ] **Step 3: Implement RTT samples + percentile**

Update `diagnostics.svelte.ts` — replace the class body to include samples and percentile logic:

```typescript
const RTT_WINDOW = 100
const MIN_RTT_SAMPLES = 10

export class Diagnostics {
  #requestsTotal = $state(0)
  #requestsErr = $state(0)
  #lastError = $state<{ reason: string; at: number } | null>(null)
  #rttSamples: number[] = []           // ring buffer; not $state — read into snapshot eagerly
  #rttRevision = $state(0)             // bump to trigger reactive re-snapshot

  constructor(_storage: Storage = globalThis.localStorage) {}

  recordRequest(durationMs: number, ok: boolean, reason?: string): void {
    this.#requestsTotal++
    if (!ok) {
      this.#requestsErr++
      this.#lastError = { reason: reason ?? 'unknown', at: Date.now() }
    }
    this.#rttSamples.push(durationMs)
    if (this.#rttSamples.length > RTT_WINDOW) this.#rttSamples.shift()
    this.#rttRevision++
  }

  snapshot(): DiagnosticsSnapshot {
    void this.#rttRevision   // read so $derived tracks it
    const total = this.#requestsTotal
    return {
      requestsTotal: total,
      requestsErr: this.#requestsErr,
      errorRate: total > 0 ? this.#requestsErr / total : 0,
      rttP50: percentile(this.#rttSamples, 0.5),
      rttP95: percentile(this.#rttSamples, 0.95),
      lastError: this.#lastError,
      fallbacks: [],
      fallbacksTotal: 0,
    }
  }
}

function percentile(samples: number[], p: number): number | null {
  if (samples.length < MIN_RTT_SAMPLES) return null
  const sorted = [...samples].sort((a, b) => a - b)
  const idx = Math.min(sorted.length - 1, Math.floor(p * sorted.length))
  return sorted[idx]
}
```

Note on the `#rttRevision` field: a plain mutable array can't be tracked by `$state`, but bumping a counter every time we mutate the array gives `snapshot()` a reactive read — Svelte's `$derived` re-runs whenever any `$state` field it touched changes.

- [ ] **Step 4: Run tests, expect all pass (9 originals + 4 new)**

```bash
cd gui/web && npm run test -- src/lib/data/__tests__/diagnostics.spec.ts
```

- [ ] **Step 5: Commit**

```bash
git add gui/web/src/lib/data/diagnostics.svelte.ts gui/web/src/lib/data/__tests__/diagnostics.spec.ts
git commit -m "feat(diagnostics): RTT ring buffer + p50/p95 percentile"
```

---

## Task 3: Diagnostics — fallback persistence (localStorage)

**Files:**
- Modify: `gui/web/src/lib/data/diagnostics.svelte.ts`
- Modify: `gui/web/src/lib/data/__tests__/diagnostics.spec.ts`

- [ ] **Step 1: Add failing tests**

Append to `diagnostics.spec.ts`:

```typescript
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
```

- [ ] **Step 2: Run, expect 6 new failures**

```bash
cd gui/web && npm run test -- src/lib/data/__tests__/diagnostics.spec.ts
```

- [ ] **Step 3: Implement persistence**

Update `diagnostics.svelte.ts`:

```typescript
const STORAGE_KEY = 'shuttle.diag.fallbacks'
const MAX_FALLBACKS = 10
const RTT_WINDOW = 100
const MIN_RTT_SAMPLES = 10

export interface DiagnosticsSnapshot { /* ...unchanged... */ }

interface FallbackEntry { reason: string; at: number }

export class Diagnostics {
  #requestsTotal = $state(0)
  #requestsErr = $state(0)
  #lastError = $state<FallbackEntry | null>(null)
  #rttSamples: number[] = []
  #rttRevision = $state(0)
  #fallbacks = $state<FallbackEntry[]>([])
  #fallbacksTotal = $state(0)

  constructor(private storage: Storage = globalThis.localStorage) {
    this.hydrate()
  }

  recordRequest(durationMs: number, ok: boolean, reason?: string): void { /* unchanged from Task 2 */ }

  recordFallback(reason: string): void {
    const entry: FallbackEntry = { reason, at: Date.now() }
    const next = [...this.#fallbacks, entry]
    this.#fallbacks = next.length > MAX_FALLBACKS ? next.slice(-MAX_FALLBACKS) : next
    this.#fallbacksTotal++
    this.persist()
  }

  snapshot(): DiagnosticsSnapshot {
    void this.#rttRevision
    const total = this.#requestsTotal
    return {
      requestsTotal: total,
      requestsErr: this.#requestsErr,
      errorRate: total > 0 ? this.#requestsErr / total : 0,
      rttP50: percentile(this.#rttSamples, 0.5),
      rttP95: percentile(this.#rttSamples, 0.95),
      lastError: this.#lastError,
      fallbacks: this.#fallbacks,
      fallbacksTotal: this.#fallbacksTotal,
    }
  }

  private hydrate(): void {
    try {
      const raw = this.storage.getItem(STORAGE_KEY)
      if (!raw) return
      const parsed = JSON.parse(raw)
      if (Array.isArray(parsed?.entries)) {
        const valid = parsed.entries.filter(
          (e: unknown): e is FallbackEntry =>
            !!e && typeof e === 'object'
            && typeof (e as any).reason === 'string'
            && typeof (e as any).at === 'number',
        )
        this.#fallbacks = valid.slice(-MAX_FALLBACKS)
      }
      if (typeof parsed?.total === 'number' && parsed.total >= 0) {
        this.#fallbacksTotal = parsed.total
      }
    } catch {
      // corrupt storage — treat as empty
    }
  }

  private persist(): void {
    try {
      this.storage.setItem(STORAGE_KEY, JSON.stringify({
        entries: this.#fallbacks,
        total: this.#fallbacksTotal,
      }))
    } catch {
      // storage disabled or quota exceeded — safe to drop
    }
  }
}

function percentile(samples: number[], p: number): number | null { /* unchanged from Task 2 */ }
```

- [ ] **Step 4: Run, expect all (15) tests pass**

```bash
cd gui/web && npm run test -- src/lib/data/__tests__/diagnostics.spec.ts
```

- [ ] **Step 5: Commit**

```bash
git add gui/web/src/lib/data/diagnostics.svelte.ts gui/web/src/lib/data/__tests__/diagnostics.spec.ts
git commit -m "feat(diagnostics): fallback history persists to localStorage"
```

---

## Task 4: Diagnostics — persistDirect static + reset

**Files:**
- Modify: `gui/web/src/lib/data/diagnostics.svelte.ts`
- Modify: `gui/web/src/lib/data/__tests__/diagnostics.spec.ts`

- [ ] **Step 1: Add failing tests**

Append:

```typescript
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
```

- [ ] **Step 2: Run, expect 5 new failures**

- [ ] **Step 3: Implement**

Add to `diagnostics.svelte.ts`:

```typescript
export class Diagnostics {
  // ...existing fields and methods...

  reset(): void {
    this.#requestsTotal = 0
    this.#requestsErr = 0
    this.#lastError = null
    this.#rttSamples = []
    this.#rttRevision++
    this.#fallbacks = []
    this.#fallbacksTotal = 0
    try {
      this.storage.removeItem(STORAGE_KEY)
    } catch {
      // ignore
    }
  }

  static persistDirect(reason: string, storage: Storage = globalThis.localStorage): void {
    try {
      const raw = storage.getItem(STORAGE_KEY)
      let entries: FallbackEntry[] = []
      let total = 0
      if (raw) {
        try {
          const parsed = JSON.parse(raw)
          if (Array.isArray(parsed?.entries)) {
            entries = parsed.entries.filter(
              (e: unknown): e is FallbackEntry =>
                !!e && typeof e === 'object'
                && typeof (e as any).reason === 'string'
                && typeof (e as any).at === 'number',
            )
          }
          if (typeof parsed?.total === 'number' && parsed.total >= 0) total = parsed.total
        } catch { /* corrupt — start fresh */ }
      }
      entries.push({ reason, at: Date.now() })
      total++
      if (entries.length > MAX_FALLBACKS) entries = entries.slice(-MAX_FALLBACKS)
      storage.setItem(STORAGE_KEY, JSON.stringify({ entries, total }))
    } catch {
      // ignore — telemetry must never break callers
    }
  }
}
```

- [ ] **Step 4: Run, expect all (20) tests pass**

```bash
cd gui/web && npm run test -- src/lib/data/__tests__/diagnostics.spec.ts
```

- [ ] **Step 5: Commit**

```bash
git add gui/web/src/lib/data/diagnostics.svelte.ts gui/web/src/lib/data/__tests__/diagnostics.spec.ts
git commit -m "feat(diagnostics): persistDirect static + reset"
```

---

## Task 5: HttpAdapter — wire diagnostics

**Files:**
- Modify: `gui/web/src/lib/data/types.ts`
- Modify: `gui/web/src/lib/data/http-adapter.ts`
- Modify: `gui/web/src/lib/data/__tests__/http-adapter.spec.ts`

- [ ] **Step 1: Add failing tests**

Append to `http-adapter.spec.ts`:

```typescript
describe('HttpAdapter — diagnostics integration', () => {
  it('exposes a diagnostics instance', () => {
    const a = new HttpAdapter()
    expect(a.diagnostics).toBeDefined()
    expect(a.diagnostics.snapshot().requestsTotal).toBe(0)
  })

  it('records a successful request', async () => {
    mockFetch(() => new Response('{}', { status: 200, headers: { 'content-type': 'application/json' } }))
    const a = new HttpAdapter()
    await a.request({ method: 'GET', path: '/x' })
    const snap = a.diagnostics.snapshot()
    expect(snap.requestsTotal).toBe(1)
    expect(snap.requestsErr).toBe(0)
    expect(snap.lastError).toBeNull()
  })

  it('records an ApiError request with status in lastError', async () => {
    mockFetch(() => new Response(JSON.stringify({ error: 'gone' }), { status: 404, headers: { 'content-type': 'application/json' } }))
    const a = new HttpAdapter()
    await expect(a.request({ method: 'GET', path: '/x' })).rejects.toBeInstanceOf(ApiError)
    const snap = a.diagnostics.snapshot()
    expect(snap.requestsTotal).toBe(1)
    expect(snap.requestsErr).toBe(1)
    expect(snap.lastError!.reason).toMatch(/gone|404/)
  })

  it('records a TransportError request with cause message in lastError', async () => {
    mockFetch(() => Promise.reject(new TypeError('fetch failed')))
    const a = new HttpAdapter()
    await expect(a.request({ method: 'GET', path: '/x' })).rejects.toBeInstanceOf(TransportError)
    const snap = a.diagnostics.snapshot()
    expect(snap.requestsErr).toBe(1)
    expect(snap.lastError!.reason).toMatch(/fetch failed/)
  })
})
```

- [ ] **Step 2: Run, expect 4 failures**

```bash
cd gui/web && npm run test -- src/lib/data/__tests__/http-adapter.spec.ts
```

- [ ] **Step 3: Add diagnostics to interface**

In `gui/web/src/lib/data/types.ts`, modify `DataAdapter`:

```typescript
import type { Diagnostics } from './diagnostics.svelte'

export interface DataAdapter {
  request<T = unknown>(opts: RequestOptions): Promise<T>
  subscribe<K extends TopicKey>(
    topic: K,
    opts?: SubscribeOptions<K>,
  ): Subscription<TopicValue<K>>
  readonly connectionState: ReadableValue<ConnectionState>
  readonly diagnostics: Diagnostics
}
```

- [ ] **Step 4: Wire into HttpAdapter**

In `gui/web/src/lib/data/http-adapter.ts`:

1. Add import: `import { Diagnostics } from './diagnostics.svelte'`
2. Add field: `readonly diagnostics = new Diagnostics()`
3. Refactor `request()` — extract the body to a private method `#requestImpl()`, wrap with timing:

```typescript
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
  // ... move existing body of request() here ...
}
```

The "AbortError treated as ok" carve-out keeps user cancellations from inflating the error rate.

- [ ] **Step 5: Run, expect tests pass**

```bash
cd gui/web && npm run test -- src/lib/data/__tests__/http-adapter.spec.ts
cd gui/web && npm run check
```

- [ ] **Step 6: Commit**

```bash
git add gui/web/src/lib/data/types.ts gui/web/src/lib/data/http-adapter.ts gui/web/src/lib/data/__tests__/http-adapter.spec.ts
git commit -m "feat(diagnostics): HttpAdapter records request timing + outcomes"
```

---

## Task 6: BridgeAdapter — wire diagnostics

**Files:**
- Modify: `gui/web/src/lib/data/bridge-adapter.ts`
- Modify: `gui/web/src/lib/data/__tests__/bridge-adapter.spec.ts`

- [ ] **Step 1: Add failing tests**

Append to `bridge-adapter.spec.ts` (mirror Task 5 cases adapted to BridgeAdapter's send mock):

```typescript
describe('BridgeAdapter — diagnostics integration', () => {
  function setBridge(send: (env: any) => Promise<any>) {
    ;(globalThis as any).window = { ShuttleBridge: { send } }
  }

  it('exposes a diagnostics instance', () => {
    setBridge(async () => ({ status: 200, body: btoa('{}'), error: null, headers: {} }))
    const a = new BridgeAdapter()
    expect(a.diagnostics.snapshot().requestsTotal).toBe(0)
  })

  it('records successful request', async () => {
    setBridge(async () => ({ status: 200, body: btoa('{}'), error: null, headers: {} }))
    const a = new BridgeAdapter()
    await a.request({ method: 'GET', path: '/x' })
    const snap = a.diagnostics.snapshot()
    expect(snap.requestsTotal).toBe(1)
    expect(snap.requestsErr).toBe(0)
  })

  it('records ApiError as error with reason', async () => {
    setBridge(async () => ({ status: 500, body: btoa(JSON.stringify({ error: 'boom' })), error: null, headers: {} }))
    const a = new BridgeAdapter()
    await expect(a.request({ method: 'GET', path: '/x' })).rejects.toBeInstanceOf(ApiError)
    const snap = a.diagnostics.snapshot()
    expect(snap.requestsErr).toBe(1)
    expect(snap.lastError!.reason).toMatch(/boom|500/)
  })

  it('records TransportError when envelope returns error', async () => {
    setBridge(async () => ({ status: -1, error: 'native send failed' }))
    const a = new BridgeAdapter()
    await expect(a.request({ method: 'GET', path: '/x' })).rejects.toBeInstanceOf(TransportError)
    const snap = a.diagnostics.snapshot()
    expect(snap.requestsErr).toBe(1)
  })
})
```

- [ ] **Step 2: Run, expect failures**

- [ ] **Step 3: Apply same wrapper pattern to BridgeAdapter**

In `gui/web/src/lib/data/bridge-adapter.ts`:

1. Add `import { Diagnostics } from './diagnostics.svelte'`
2. Add `readonly diagnostics = new Diagnostics()`
3. Extract current `request()` body to `#requestImpl()`, add the same timing wrapper as HttpAdapter (with the same AbortError carve-out)

- [ ] **Step 4: Run tests**

```bash
cd gui/web && npm run test -- src/lib/data/__tests__/bridge-adapter.spec.ts
cd gui/web && npm run check
```

- [ ] **Step 5: Commit**

```bash
git add gui/web/src/lib/data/bridge-adapter.ts gui/web/src/lib/data/__tests__/bridge-adapter.spec.ts
git commit -m "feat(diagnostics): BridgeAdapter records request timing + outcomes"
```

---

## Task 7: Conformance test — adapters track identically

**Files:**
- Modify: `gui/web/src/lib/data/__tests__/conformance.spec.ts`

- [ ] **Step 1: Add cross-adapter test**

In `conformance.spec.ts`, find the existing `describe.each(factories)` block (look for `'http'` and `'bridge'` factories) and add a new block at the same level:

```typescript
describe.each(factories)('%s adapter — diagnostics conformance', (_label, factory) => {
  it('reports identical counts after the same workload', async () => {
    const a = await factory()
    // Mock fetch for both adapters (bridge factory routes through fetch too)
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
```

- [ ] **Step 2: Run**

```bash
cd gui/web && npm run test -- src/lib/data/__tests__/conformance.spec.ts
```
Expected: PASS for both `http` and `bridge` factories.

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/lib/data/__tests__/conformance.spec.ts
git commit -m "test(diagnostics): conformance — adapters track identically"
```

---

## Task 8: Adapter accessor — tryGetAdapter helper

**Files:**
- Modify: `gui/web/src/lib/data/index.ts`

- [ ] **Step 1: Add tryGetAdapter export**

In `gui/web/src/lib/data/index.ts`:

```typescript
// gui/web/src/lib/data/index.ts
import type { DataAdapter } from './types'

let _adapter: DataAdapter | null = null

export function setAdapter(a: DataAdapter): void { _adapter = a }

export function getAdapter(): DataAdapter {
  if (!_adapter) throw new Error('DataAdapter not initialised — call setAdapter() during boot')
  return _adapter
}

/** Returns null instead of throwing — for early-boot paths that may run before setAdapter. */
export function tryGetAdapter(): DataAdapter | null {
  return _adapter
}

/** Test-only helper. */
export function __resetAdapter(): void { _adapter = null }
```

- [ ] **Step 2: Verify nothing breaks**

```bash
cd gui/web && npm run test
cd gui/web && npm run check
```

- [ ] **Step 3: Commit**

```bash
git add gui/web/src/lib/data/index.ts
git commit -m "feat(data): tryGetAdapter — null-safe accessor for early boot"
```

---

## Task 9: boot.ts — record fallback before postMessage

**Files:**
- Modify: `gui/web/src/app/boot.ts`
- Modify: `gui/web/src/app/__tests__/boot.spec.ts`

- [ ] **Step 1: Add failing tests**

In `boot.spec.ts`, append:

```typescript
describe('boot — fallback diagnostics recording', () => {
  beforeEach(() => {
    __resetAdapter()
    delete (window as any).ShuttleBridge
    delete (window as any).webkit
    Object.defineProperty(window, 'location', { value: { search: '' }, writable: true })
    localStorage.clear()
  })

  it('persists fallback to localStorage before postMessage when probe fails', async () => {
    const post = vi.fn()
    ;(window as any).webkit = { messageHandlers: { fallback: { postMessage: post } } }
    ;(window as any).ShuttleBridge = { send: async () => { throw new Error('unreachable') } }

    let storedAtPostTime: string | null = null
    post.mockImplementation(() => {
      storedAtPostTime = localStorage.getItem('shuttle.diag.fallbacks')
    })

    await boot()
    expect(storedAtPostTime).toBeTruthy()
    const parsed = JSON.parse(storedAtPostTime!)
    expect(parsed.entries.length).toBeGreaterThan(0)
    expect(parsed.entries[0].reason).toMatch(/unreachable/i)
  })

  it('writes via persistDirect when ShuttleBridge is missing under bridge=1', async () => {
    Object.defineProperty(window, 'location', { value: { search: '?bridge=1' }, writable: true })
    const post = vi.fn()
    ;(window as any).webkit = { messageHandlers: { fallback: { postMessage: post } } }

    await boot()
    expect(post).toHaveBeenCalled()
    const stored = localStorage.getItem('shuttle.diag.fallbacks')
    expect(stored).toBeTruthy()
    const parsed = JSON.parse(stored!)
    expect(parsed.total).toBeGreaterThanOrEqual(1)
  })
})
```

- [ ] **Step 2: Run, expect 2 failures**

```bash
cd gui/web && npm run test -- src/app/__tests__/boot.spec.ts
```

- [ ] **Step 3: Update boot.ts**

Replace `gui/web/src/app/boot.ts`:

```typescript
// gui/web/src/app/boot.ts
import { setAdapter, tryGetAdapter } from '@/lib/data'
import { HttpAdapter } from '@/lib/data/http-adapter'
import { BridgeAdapter } from '@/lib/data/bridge-adapter'
import { Diagnostics } from '@/lib/data/diagnostics.svelte'
import type { DataAdapter } from '@/lib/data/types'

declare global {
  interface Window {
    webkit?: {
      messageHandlers?: {
        fallback?: { postMessage: (msg: unknown) => void }
      }
    }
  }
}

function timeout(ms: number): Promise<never> {
  return new Promise((_resolve, reject) => setTimeout(() => reject(new Error('timeout')), ms))
}

function requestFallback(reason: string, adapter?: DataAdapter | null): void {
  if (typeof window === 'undefined') return
  // CRITICAL: persist BEFORE postMessage. The Swift FallbackHandler will
  // tear down this WKWebView in response to the message, which blows away
  // the in-memory adapter. localStorage writes are synchronous on iOS WKWebView.
  try {
    if (adapter) {
      adapter.diagnostics.recordFallback(reason)
    } else {
      Diagnostics.persistDirect(reason)
    }
  } catch {
    // never block fallback on telemetry
  }
  window.webkit?.messageHandlers?.fallback?.postMessage({ reason, timestamp: Date.now() })
}

export async function boot(): Promise<void> {
  const force = typeof location !== 'undefined'
    ? new URLSearchParams(location.search).get('bridge')
    : null

  if (force === '0') {
    setAdapter(new HttpAdapter())
    return
  }

  if (typeof window !== 'undefined' && !window.ShuttleBridge) {
    await new Promise((r) => setTimeout(r, 100))
  }

  if (typeof window === 'undefined' || !window.ShuttleBridge) {
    if (force === '1') {
      requestFallback('ShuttleBridge missing under bridge=1 force flag')
      return
    }
    setAdapter(new HttpAdapter())
    return
  }

  const bridge = new BridgeAdapter()
  try {
    await Promise.race([
      bridge.request({ method: 'GET', path: '/api/healthz', timeoutMs: 5000 }),
      timeout(5000),
    ])
    setAdapter(bridge)
  } catch (err) {
    if (force === '1') {
      setAdapter(bridge)
      return
    }
    const reason = String(err instanceof Error ? err.message : err)
    requestFallback(reason, bridge)   // bridge instance carries diagnostics
    return
  }

  window.addEventListener?.('unhandledrejection', (ev) => {
    if (typeof ev.reason === 'object' && ev.reason && String(ev.reason).includes('[bridge-fatal]')) {
      requestFallback(String(ev.reason), tryGetAdapter())
    }
  })
}
```

- [ ] **Step 4: Run, expect tests pass**

```bash
cd gui/web && npm run test -- src/app/__tests__/boot.spec.ts
cd gui/web && npm run check
```

- [ ] **Step 5: Commit**

```bash
git add gui/web/src/app/boot.ts gui/web/src/app/__tests__/boot.spec.ts
git commit -m "feat(boot): record fallback to diagnostics before postMessage"
```

---

## Task 10: i18n — diag namespace strings

**Files:**
- Modify: `gui/web/src/locales/en.json`
- Modify: `gui/web/src/locales/zh-CN.json`

- [ ] **Step 1: Add to en.json**

In `gui/web/src/locales/en.json`, locate the `"settings"` block. Inside `"settings"`:

1. Inside `"nav"` object, add `"diag": "Diagnostics"` (alongside `general`, `proxy`, etc.)
2. At the top level of `"settings"`, add a new `"diag"` object:

```json
"diag": {
  "title": "Diagnostics",
  "subtitle": "Bridge transport health and fallback history. Local data only.",
  "section": {
    "bridgeHealth": "Bridge health",
    "lastError": "Last error",
    "fallbackHistory": "Fallback history"
  },
  "stat": {
    "requests": "Requests",
    "errors": "Errors",
    "errorRate": "Error rate",
    "rttP50": "RTT p50",
    "rttP95": "RTT p95"
  },
  "empty": {
    "noSamples": "Not enough samples yet",
    "noErrors": "No errors recorded this session",
    "noFallbacks": "No fallback events recorded"
  },
  "action": {
    "reset": "Reset counters",
    "confirmReset": "Clear all diagnostic data?"
  },
  "totalTriggers": "Total triggers: {count}",
  "relativeJustNow": "just now",
  "rttUnit": "ms"
}
```

- [ ] **Step 2: Mirror in zh-CN.json**

In `gui/web/src/locales/zh-CN.json`:

1. Inside `settings.nav`, add `"diag": "诊断"`
2. Add the `diag` block at the top level of `settings`:

```json
"diag": {
  "title": "诊断",
  "subtitle": "Bridge 传输健康状况与回退历史。所有数据仅保存在本机。",
  "section": {
    "bridgeHealth": "Bridge 健康",
    "lastError": "最近一次错误",
    "fallbackHistory": "回退历史"
  },
  "stat": {
    "requests": "请求数",
    "errors": "错误数",
    "errorRate": "错误率",
    "rttP50": "RTT p50",
    "rttP95": "RTT p95"
  },
  "empty": {
    "noSamples": "样本数量不足",
    "noErrors": "本次会话无错误",
    "noFallbacks": "无回退记录"
  },
  "action": {
    "reset": "清空计数器",
    "confirmReset": "确定要清空所有诊断数据吗？"
  },
  "totalTriggers": "累计触发: {count}",
  "relativeJustNow": "刚才",
  "rttUnit": "毫秒"
}
```

- [ ] **Step 3: Verify type-check still passes**

```bash
cd gui/web && npm run check
```

i18n in this project derives types from `en.json` (see `lib/i18n/index.ts`); zh-CN must structurally match.

- [ ] **Step 4: Commit**

```bash
git add gui/web/src/locales/en.json gui/web/src/locales/zh-CN.json
git commit -m "i18n(diag): add settings.diag.* namespace (en + zh-CN)"
```

---

## Task 11: Diagnostics.svelte UI

**Files:**
- Create: `gui/web/src/features/settings/sub/Diagnostics.svelte`

- [ ] **Step 1: Inspect a sibling sub-page for layout conventions**

Open `gui/web/src/features/settings/sub/Logging.svelte` and `gui/web/src/features/settings/sub/Qos.svelte` to confirm: PageHeader use, `--shuttle-*` token reliance, `t()` call pattern.

- [ ] **Step 2: Write the component**

Create `gui/web/src/features/settings/sub/Diagnostics.svelte`:

```svelte
<script lang="ts">
  import PageHeader from '../PageHeader.svelte'
  import { t } from '@/lib/i18n/index'
  import { getAdapter } from '@/lib/data'
  import type { DiagnosticsSnapshot } from '@/lib/data/diagnostics.svelte'

  const adapter = getAdapter()
  const snap = $derived<DiagnosticsSnapshot>(adapter.diagnostics.snapshot())

  const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })
  const dtf = new Intl.DateTimeFormat(undefined, { dateStyle: 'short', timeStyle: 'medium' })

  function relativeTime(at: number): string {
    const diff = Date.now() - Math.min(at, Date.now())
    if (diff < 60_000) return t('settings.diag.relativeJustNow')
    if (diff < 3_600_000) return rtf.format(-Math.floor(diff / 60_000), 'minute')
    if (diff < 86_400_000) return rtf.format(-Math.floor(diff / 3_600_000), 'hour')
    return rtf.format(-Math.floor(diff / 86_400_000), 'day')
  }

  function fmtPct(v: number): string {
    return `${(v * 100).toFixed(2)}%`
  }

  function fmtRtt(v: number | null): string {
    return v === null ? '—' : `${Math.round(v)} ${t('settings.diag.rttUnit')}`
  }

  function onResetClick() {
    if (confirm(t('settings.diag.action.confirmReset'))) {
      adapter.diagnostics.reset()
    }
  }
</script>

<PageHeader title={t('settings.diag.title')} description={t('settings.diag.subtitle')} />

<section class="block">
  <h3 class="head">{t('settings.diag.section.bridgeHealth')}</h3>
  <dl class="stats">
    <div><dt>{t('settings.diag.stat.requests')}</dt><dd>{snap.requestsTotal.toLocaleString()}</dd></div>
    <div>
      <dt>{t('settings.diag.stat.errors')}</dt>
      <dd>
        {snap.requestsErr.toLocaleString()}
        {#if snap.requestsTotal > 0}<span class="muted">({fmtPct(snap.errorRate)})</span>{/if}
      </dd>
    </div>
    <div>
      <dt>{t('settings.diag.stat.rttP50')}</dt>
      <dd>
        {#if snap.rttP50 === null}
          <span title={t('settings.diag.empty.noSamples')}>—</span>
        {:else}{fmtRtt(snap.rttP50)}{/if}
      </dd>
    </div>
    <div>
      <dt>{t('settings.diag.stat.rttP95')}</dt>
      <dd>
        {#if snap.rttP95 === null}
          <span title={t('settings.diag.empty.noSamples')}>—</span>
        {:else}{fmtRtt(snap.rttP95)}{/if}
      </dd>
    </div>
  </dl>
</section>

<section class="block">
  <h3 class="head">{t('settings.diag.section.lastError')}</h3>
  {#if snap.lastError}
    <div class="card">
      <code class="reason">{snap.lastError.reason}</code>
      <div class="when">{relativeTime(snap.lastError.at)}</div>
    </div>
  {:else}
    <div class="empty">{t('settings.diag.empty.noErrors')}</div>
  {/if}
</section>

<section class="block">
  <h3 class="head">{t('settings.diag.section.fallbackHistory')}</h3>
  {#if snap.fallbacks.length === 0}
    <div class="empty">{t('settings.diag.empty.noFallbacks')}</div>
  {:else}
    <ul class="list">
      {#each [...snap.fallbacks].reverse() as entry (entry.at + entry.reason)}
        <li>
          <code class="reason">{entry.reason}</code>
          <span class="when">{dtf.format(new Date(entry.at))}</span>
        </li>
      {/each}
    </ul>
    <div class="muted total">{t('settings.diag.totalTriggers').replace('{count}', String(snap.fallbacksTotal))}</div>
  {/if}
</section>

<section class="block">
  <button type="button" class="reset" onclick={onResetClick}>{t('settings.diag.action.reset')}</button>
</section>

<style>
  .block { margin-bottom: var(--shuttle-space-6); }
  .head {
    font-size: var(--shuttle-text-sm);
    font-weight: 600;
    color: var(--shuttle-fg-secondary);
    margin: 0 0 var(--shuttle-space-3);
  }
  .stats {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--shuttle-space-3);
    margin: 0;
  }
  .stats > div {
    background: var(--shuttle-bg-subtle);
    padding: var(--shuttle-space-3);
    border-radius: var(--shuttle-radius-md);
  }
  .stats dt { font-size: var(--shuttle-text-xs); color: var(--shuttle-fg-muted); margin-bottom: 2px; }
  .stats dd { font-size: var(--shuttle-text-lg); font-weight: 500; margin: 0; }
  .muted { color: var(--shuttle-fg-muted); font-size: var(--shuttle-text-xs); margin-left: var(--shuttle-space-2); }
  .total { margin: var(--shuttle-space-2) 0 0; }
  .card, .empty {
    background: var(--shuttle-bg-subtle);
    padding: var(--shuttle-space-3);
    border-radius: var(--shuttle-radius-md);
  }
  .reason {
    font-family: var(--shuttle-font-mono);
    font-size: var(--shuttle-text-sm);
    word-break: break-all;
  }
  .when { color: var(--shuttle-fg-muted); font-size: var(--shuttle-text-xs); margin-top: 2px; }
  .empty { color: var(--shuttle-fg-muted); font-size: var(--shuttle-text-sm); }
  .list { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: var(--shuttle-space-2); }
  .list li {
    display: flex;
    justify-content: space-between;
    align-items: center;
    background: var(--shuttle-bg-subtle);
    padding: var(--shuttle-space-2) var(--shuttle-space-3);
    border-radius: var(--shuttle-radius-sm);
  }
  .reset {
    background: transparent;
    border: 1px solid var(--shuttle-border);
    color: var(--shuttle-fg-primary);
    padding: var(--shuttle-space-2) var(--shuttle-space-4);
    border-radius: var(--shuttle-radius-md);
    font-size: var(--shuttle-text-sm);
    cursor: pointer;
  }
  .reset:hover { background: var(--shuttle-bg-subtle); }
</style>
```

- [ ] **Step 3: Type-check**

```bash
cd gui/web && npm run check
```

If `PageHeader.svelte` doesn't accept a `description` prop, drop that attribute. Verify by reading `gui/web/src/features/settings/PageHeader.svelte` first.

- [ ] **Step 4: Commit**

```bash
git add gui/web/src/features/settings/sub/Diagnostics.svelte
git commit -m "feat(diagnostics): Settings → Diagnostics sub-page UI"
```

---

## Task 12: Wire sub-page into settings nav

**Files:**
- Modify: `gui/web/src/features/settings/nav.ts`
- Modify: `gui/web/src/features/settings/SettingsPage.svelte`

- [ ] **Step 1: Add nav entry**

In `gui/web/src/features/settings/nav.ts`, add `diag` entry **first** in the `diagnostics` section (before `logging`):

```typescript
export const subNav: SubNavEntry[] = [
  // Basics
  { slug: 'general',  labelKey: 'settings.nav.general',  icon: 'settings',  section: 'basics' },
  { slug: 'proxy',    labelKey: 'settings.nav.proxy',    icon: 'servers',   section: 'basics' },
  { slug: 'update',   labelKey: 'settings.nav.update',   icon: 'refresh',   section: 'basics' },

  // Network
  { slug: 'mesh',     labelKey: 'settings.nav.mesh',     icon: 'mesh',      section: 'network' },
  { slug: 'routing',  labelKey: 'settings.nav.routing',  icon: 'routing',   section: 'network' },
  { slug: 'dns',      labelKey: 'settings.nav.dns',      icon: 'globe',     section: 'network' },

  // Diagnostics
  { slug: 'diag',     labelKey: 'settings.nav.diag',     icon: 'activity',  section: 'diagnostics' },
  { slug: 'logging',  labelKey: 'settings.nav.logging',  icon: 'logs',      section: 'diagnostics' },
  { slug: 'qos',      labelKey: 'settings.nav.qos',      icon: 'zap',       section: 'diagnostics' },

  // Advanced
  { slug: 'backup',   labelKey: 'settings.nav.backup',   icon: 'download',  section: 'advanced' },
  { slug: 'advanced', labelKey: 'settings.nav.advanced', icon: 'wrench',    section: 'advanced' },
]
```

- [ ] **Step 2: Wire pageMap**

In `gui/web/src/features/settings/SettingsPage.svelte`, add the import and route entry:

```svelte
<script lang="ts">
  // ...existing imports...
  import Diagnostics from './sub/Diagnostics.svelte'
  // ...

  const pageMap: Record<string, Component> = {
    general: General, proxy: Proxy, mesh: Mesh, routing: Routing, dns: Dns,
    diag: Diagnostics,
    logging: Logging, qos: Qos, backup: Backup, update: Update, advanced: Advanced,
  }
  // ...
</script>
```

- [ ] **Step 3: Verify type-check + tests still green**

```bash
cd gui/web && npm run check
cd gui/web && npm run test
```

- [ ] **Step 4: Commit**

```bash
git add gui/web/src/features/settings/nav.ts gui/web/src/features/settings/SettingsPage.svelte
git commit -m "feat(settings): mount Diagnostics sub-page in nav"
```

---

## Task 13: UI component test

**Files:**
- Create: `gui/web/src/features/settings/sub/__tests__/Diagnostics.test.ts`

- [ ] **Step 1: Find existing Svelte test pattern**

Skim `gui/web/src/app/Sidebar.test.ts` or `gui/web/src/app/TopBar.test.ts` for the project's `@testing-library/svelte` style.

- [ ] **Step 2: Write tests**

Create `gui/web/src/features/settings/sub/__tests__/Diagnostics.test.ts`:

```typescript
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import Diagnostics from '../Diagnostics.svelte'
import { setAdapter, __resetAdapter } from '@/lib/data'
import { Diagnostics as DiagClass } from '@/lib/data/diagnostics.svelte'
import type { DataAdapter } from '@/lib/data/types'

function makeStorage(): Storage {
  const m = new Map<string, string>()
  return {
    get length() { return m.size },
    clear() { m.clear() },
    getItem: (k) => m.get(k) ?? null,
    setItem: (k, v) => { m.set(k, v) },
    removeItem: (k) => { m.delete(k) },
    key: (i) => [...m.keys()][i] ?? null,
  }
}

function fakeAdapter(): DataAdapter {
  const diag = new DiagClass(makeStorage())
  return {
    diagnostics: diag,
    request: vi.fn(),
    subscribe: vi.fn() as any,
    connectionState: { value: 'idle', subscribe: () => () => {} },
  }
}

describe('Diagnostics.svelte', () => {
  beforeEach(() => { __resetAdapter() })
  afterEach(() => { __resetAdapter() })

  it('renders empty states when no data', () => {
    setAdapter(fakeAdapter())
    render(Diagnostics)
    expect(screen.getByText(/No errors recorded/i)).toBeInTheDocument()
    expect(screen.getByText(/No fallback events/i)).toBeInTheDocument()
  })

  it('shows — for RTT when fewer than 10 samples', () => {
    const a = fakeAdapter()
    a.diagnostics.recordRequest(20, true)
    setAdapter(a)
    render(Diagnostics)
    const dashes = screen.getAllByText('—')
    expect(dashes.length).toBeGreaterThanOrEqual(2)   // p50 + p95
  })

  it('shows lastError reason and relative time when present', () => {
    const a = fakeAdapter()
    a.diagnostics.recordRequest(50, false, 'TransportError: timeout')
    setAdapter(a)
    render(Diagnostics)
    expect(screen.getByText(/TransportError: timeout/)).toBeInTheDocument()
  })

  it('renders fallback list most-recent-first', () => {
    const a = fakeAdapter()
    a.diagnostics.recordFallback('first')
    a.diagnostics.recordFallback('second')
    setAdapter(a)
    render(Diagnostics)
    const items = screen.getAllByText(/first|second/)
    // 'second' (newest) should appear before 'first' in DOM order
    const html = document.body.innerHTML
    expect(html.indexOf('second')).toBeLessThan(html.indexOf('first'))
  })

  it('reset button clears state when confirmed', async () => {
    const a = fakeAdapter()
    a.diagnostics.recordFallback('boom')
    setAdapter(a)
    vi.spyOn(window, 'confirm').mockReturnValueOnce(true)
    render(Diagnostics)
    const btn = screen.getByRole('button', { name: /Reset counters/i })
    btn.click()
    // After click, snapshot should be empty — re-render via $derived flush
    await Promise.resolve()
    expect(a.diagnostics.snapshot().fallbacks).toHaveLength(0)
  })
})
```

- [ ] **Step 3: Run**

```bash
cd gui/web && npm run test -- src/features/settings/sub/__tests__/Diagnostics.test.ts
```

- [ ] **Step 4: Commit**

```bash
git add gui/web/src/features/settings/sub/__tests__/Diagnostics.test.ts
git commit -m "test(diagnostics): UI component renders + reset behavior"
```

---

## Task 14: Full test suite + type-check sanity

- [ ] **Step 1: Run full GUI test suite**

```bash
cd gui/web && npm run test
```

Expected: all green (existing + new tests).

- [ ] **Step 2: Run type-check**

```bash
cd gui/web && npm run check
```

Expected: zero errors.

- [ ] **Step 3: Run host-safe Go tests** (no Go was modified, but project guard rule says always use the script)

```bash
./scripts/test.sh
```

Expected: green. (No Go changes in this branch — this is a regression sanity gate.)

- [ ] **Step 4: If anything fails**

Fix in place, re-run. Do NOT commit a regression.

- [ ] **Step 5: Commit any cleanup if needed (likely no commit needed)**

---

## Task 15: PR handoff

- [ ] **Step 1: Push branch**

```bash
git push -u origin feat/diagnostics-panel
```

- [ ] **Step 2: Open PR**

```bash
gh pr create --title "feat(diagnostics): Settings → Diagnostics panel via DataAdapter" --body "$(cat <<'EOF'
## Summary

Implements the Settings → Diagnostics sub-page (Phase β prerequisite per
`docs/superpowers/specs/2026-04-24-ios-vpn-mode-spa-design.md` §11.3) by
injecting a `Diagnostics` instance into the existing `DataAdapter`
abstraction. One implementation, one UI, all platforms.

- New `Diagnostics` class tracks request count / errors / RTT p50/p95
  in memory and persists fallback history to localStorage.
- Both `HttpAdapter` and `BridgeAdapter` wrap their `request()` in a
  timing try/finally that feeds the tracker.
- `boot.ts` records fallback to `localStorage` synchronously **before**
  posting the message that tears down the SPA, so the entry survives.
- New Settings sub-page `/settings/diag` renders the snapshot reactively.

Diverges from §11.3's UserDefaults approach — see spec §8 for rationale
(localStorage is sync, cross-platform, and same-tier as the rest of the
SPA's persistence).

## Test plan

- [ ] `cd gui/web && npm run test` — all green (~38 new test cases)
- [ ] `cd gui/web && npm run check` — type-check clean
- [ ] `./scripts/test.sh` — Go regression gate green
- [ ] Manual: `cd gui/web && npm run dev` → open Settings → Diagnostics, verify empty state renders
- [ ] Manual: trigger a few requests via DevTools, verify counters increment
- [ ] Manual: click Reset, confirm dialog, verify state clears

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 3: Note PR URL for handoff**

---

## Self-Review Notes

**Spec coverage check:**
- §3 architecture (DataAdapter + Diagnostics) → Tasks 1–6
- §4.1 Diagnostics class fields & methods → Tasks 1–4
- §4.2 DataAdapter interface change → Task 5 step 3
- §4.3 request timing wrapper → Tasks 5–6
- §4.4 boot.ts integration + persistDirect routing → Tasks 8–9
- §4.5 UI layout → Task 11
- §5 data flow (sync write before postMessage) → Task 9 step 1 test
- §6 error handling (corrupt JSON, quota, etc.) → Task 3 tests
- §7 testing matrix → Tasks 1–7, 9, 13
- §8 spec amendment note → captured in PR body
- i18n (settings.diag.*) → Task 10
- nav entry → Task 12

**Type consistency:** `Diagnostics` class name, `recordRequest`/`recordFallback`/`snapshot`/`reset` method names, `DiagnosticsSnapshot` field names, `STORAGE_KEY` value `'shuttle.diag.fallbacks'`, `MAX_FALLBACKS=10`, `RTT_WINDOW=100`, `MIN_RTT_SAMPLES=10` — all consistent across tasks and tests.

**No placeholders detected.**
