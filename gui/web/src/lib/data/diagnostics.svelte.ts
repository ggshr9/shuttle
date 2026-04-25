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
    // _durationMs is wired into the RTT ring buffer in Task 2.
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
