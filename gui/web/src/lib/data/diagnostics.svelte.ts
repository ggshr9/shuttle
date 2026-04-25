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

const RTT_WINDOW = 100
const MIN_RTT_SAMPLES = 10

export class Diagnostics {
  #requestsTotal = $state(0)
  #requestsErr = $state(0)
  #lastError = $state<{ reason: string; at: number } | null>(null)
  #rttSamples: number[] = []           // ring buffer; not $state — read into snapshot eagerly
  #rttRevision = $state(0)             // bump to trigger reactive re-snapshot

  constructor(_storage: Storage = globalThis.localStorage) {
    // storage usage lands in Task 3
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
  // Uses upper-index variant: floor(p * N) instead of strict nearest-rank
  // ceil(p * N) - 1. The 1-element difference is immaterial for RTT display.
  const sorted = [...samples].sort((a, b) => a - b)
  const idx = Math.min(sorted.length - 1, Math.floor(p * sorted.length))
  return sorted[idx]
}
