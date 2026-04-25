// gui/web/src/lib/data/internal/json.ts

/**
 * Parses JSON, falling back to the raw string on parse failure. On the
 * success (2xx) path callers consume the return as T; a raw-string return
 * would indicate a server contract violation. On the error (4xx/5xx) path
 * the ApiError extraction guards against non-object parsed shapes, so the
 * fallback is safe there.
 */
export function safeJson(s: string): unknown {
  try { return JSON.parse(s) } catch { return s }
}
