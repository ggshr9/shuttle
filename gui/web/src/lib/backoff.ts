// Capped exponential backoff with multiplicative jitter, used by all
// reconnect/retry loops on the frontend so a flapping backend doesn't
// see synchronized client stampedes.
//
// Attempt 0 returns ~baseMs (with jitter); each subsequent attempt
// doubles, clamped to maxMs. Jitter is ±25% of the computed delay.

export interface BackoffOptions {
  baseMs?: number
  maxMs?: number
  jitter?: number // 0..1; 0.25 = ±25%
}

const DEFAULT_BASE_MS = 2_000
const DEFAULT_MAX_MS = 30_000
const DEFAULT_JITTER = 0.25

export function nextBackoffMs(attempt: number, opts: BackoffOptions = {}): number {
  const base = opts.baseMs ?? DEFAULT_BASE_MS
  const max = opts.maxMs ?? DEFAULT_MAX_MS
  const jitter = opts.jitter ?? DEFAULT_JITTER

  const exp = Math.min(max, base * 2 ** Math.max(0, attempt))
  // Multiplicative jitter in [1 - jitter, 1 + jitter].
  const factor = 1 + (Math.random() * 2 - 1) * jitter
  return Math.round(exp * factor)
}
