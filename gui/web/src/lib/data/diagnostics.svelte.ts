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

const STORAGE_KEY = 'shuttle.diag.fallbacks'
const MAX_FALLBACKS = 10
const RTT_WINDOW = 100
const MIN_RTT_SAMPLES = 10

interface FallbackEntry { reason: string; at: number }

export class Diagnostics {
  #requestsTotal = $state(0)
  #requestsErr = $state(0)
  #lastError = $state<FallbackEntry | null>(null)
  #rttSamples: number[] = []           // ring buffer; not $state — read into snapshot eagerly
  #rttRevision = $state(0)             // bump to trigger reactive re-snapshot
  #fallbacks = $state<FallbackEntry[]>([])
  #fallbacksTotal = $state(0)

  constructor(private storage: Storage = globalThis.localStorage) {
    this.hydrate()
  }

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

  recordFallback(reason: string): void {
    // Always replaces #fallbacks (never mutates in-place) so callers holding
    // a snapshot reference see a stable, frozen array.
    const entry: FallbackEntry = { reason, at: Date.now() }
    const next = [...this.#fallbacks, entry]
    this.#fallbacks = next.length > MAX_FALLBACKS ? next.slice(-MAX_FALLBACKS) : next
    this.#fallbacksTotal++
    this.persist()
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

function percentile(samples: number[], p: number): number | null {
  if (samples.length < MIN_RTT_SAMPLES) return null
  // Uses upper-index variant: floor(p * N) instead of strict nearest-rank
  // ceil(p * N) - 1. The 1-element difference is immaterial for RTT display.
  const sorted = [...samples].sort((a, b) => a - b)
  const idx = Math.min(sorted.length - 1, Math.floor(p * sorted.length))
  return sorted[idx]
}
