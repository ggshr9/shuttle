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
  // CRITICAL: persist BEFORE postMessage. The Swift FallbackHandler tears
  // down this WKWebView in response to the message, blowing away the
  // in-memory adapter. localStorage writes are synchronous on iOS WKWebView.
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
