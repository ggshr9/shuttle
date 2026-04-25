// gui/web/src/app/boot.ts
import { setAdapter } from '@/lib/data'
import { HttpAdapter } from '@/lib/data/http-adapter'
import { BridgeAdapter } from '@/lib/data/bridge-adapter'

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
  return new Promise((_resolve, reject) =>
    setTimeout(() => reject(new Error('timeout')), ms),
  )
}

function requestFallback(reason: string): void {
  if (typeof window === 'undefined') return
  window.webkit?.messageHandlers?.fallback?.postMessage({
    reason,
    timestamp: Date.now(),
  })
}

export async function boot(): Promise<void> {
  const force = typeof location !== 'undefined'
    ? new URLSearchParams(location.search).get('bridge')
    : null

  // ?bridge=0 — force HTTP path even if bridge is present. Debug only.
  if (force === '0') {
    setAdapter(new HttpAdapter())
    return
  }

  // Wait briefly for the user-script-injected window.ShuttleBridge to appear.
  // The script is injected at document start so it should be there before any
  // module code, but defensively allow a short window for race conditions.
  if (typeof window !== 'undefined' && !window.ShuttleBridge) {
    await new Promise((r) => setTimeout(r, 100))
  }

  // No bridge present → not iOS VPN mode → install HttpAdapter.
  if (typeof window === 'undefined' || !window.ShuttleBridge) {
    if (force === '1') {
      requestFallback('ShuttleBridge missing under bridge=1 force flag')
      return
    }
    setAdapter(new HttpAdapter())
    return
  }

  // Bridge present — probe healthz before installing.
  const bridge = new BridgeAdapter()
  try {
    await Promise.race([
      bridge.request({ method: 'GET', path: '/api/healthz', timeoutMs: 5000 }),
      timeout(5000),
    ])
    setAdapter(bridge)
  } catch (err) {
    requestFallback(String(err instanceof Error ? err.message : err))
  }

  // Tagged unhandled-rejection escape hatch — adapter code can throw with
  // [bridge-fatal] in the message to force a fallback signal.
  window.addEventListener?.('unhandledrejection', (ev) => {
    if (typeof ev.reason === 'object' && ev.reason && String(ev.reason).includes('[bridge-fatal]')) {
      requestFallback(String(ev.reason))
    }
  })
}
