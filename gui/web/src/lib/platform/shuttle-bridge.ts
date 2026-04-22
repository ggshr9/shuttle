// Promise-based wrapper for WebView ↔ native messaging.
//
// Two transport styles are supported:
// (1) Single-invoke: native injects a `window.ShuttleVPN.invoke(jsonStr)`
//     function. The wrapper generates a numeric id, posts
//     `{ id, action, payload }`, and waits for the native side to call
//     `window._shuttleResolve(id, value)` or `window._shuttleReject(id, msg)`.
// (2) Per-method: native injects one function per action on
//     `window.ShuttleVPN` (the original Phase 1 style — `start()`, `stop()`,
//     `isRunning()`, `getStatus()`). Older bridge binaries pre-date invoke;
//     the fallback path here keeps them working.
//
// New code should prefer (1). Phase 4 Android/iOS native extensions switch
// to invoke-based dispatch so request/response plumbing is consistent.

interface ShuttleBridge {
  invoke?: (msg: string) => void
  [method: string]: unknown
}

let counter = 0
const pending = new Map<number, { resolve: (v: unknown) => void; reject: (e: unknown) => void }>()

function bridge(): ShuttleBridge | null {
  if (typeof window === 'undefined') return null
  return ((window as unknown) as { ShuttleVPN?: ShuttleBridge }).ShuttleVPN ?? null
}

function installGlobalHandlers() {
  if (typeof window === 'undefined') return
  const w = window as unknown as {
    _shuttleResolve?: (id: number, value: unknown) => void
    _shuttleReject?: (id: number, err: string) => void
  }
  w._shuttleResolve = (id: number, value: unknown) => {
    const p = pending.get(id); if (!p) return
    pending.delete(id); p.resolve(value)
  }
  w._shuttleReject = (id: number, err: string) => {
    const p = pending.get(id); if (!p) return
    pending.delete(id); p.reject(new Error(err))
  }
}

installGlobalHandlers()

export function callBridge<T>(action: string, payload?: unknown): Promise<T> {
  // Re-install global handlers in case they were removed (e.g., between tests).
  installGlobalHandlers()
  const b = bridge()
  if (!b) return Promise.reject(new Error('Shuttle bridge not available'))

  // Prefer invoke-style dispatch when available.
  if (typeof b.invoke === 'function') {
    return new Promise<T>((resolve, reject) => {
      const id = ++counter
      pending.set(id, {
        resolve: resolve as (v: unknown) => void,
        reject,
      })
      try {
        b.invoke!(JSON.stringify({ id, action, payload }))
      } catch (e) {
        pending.delete(id)
        reject(e)
      }
    })
  }

  // Fallback: direct method call on the bridge. Native binaries from
  // Phase 1 exposed methods directly (no invoke shim); keep them working.
  const fn = b[action]
  if (typeof fn === 'function') {
    try {
      const result = (fn as (p?: unknown) => unknown)(payload)
      return Promise.resolve(result as T)
    } catch (e) {
      return Promise.reject(e)
    }
  }

  return Promise.reject(new Error(`bridge method "${action}" not available`))
}

// Test helper — clear pending map + counter between tests so id collisions
// can't leak. Not exported publicly.
export function __resetBridge(): void {
  pending.clear()
  counter = 0
}
