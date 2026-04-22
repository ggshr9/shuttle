// Dev-only mock for window.ShuttleVPN, activated by running
//   npm run dev
// and visiting http://localhost:5173/?mockbridge=1
//
// Simulates the Phase 4 invoke-based native bridge so Scan-QR / share /
// openExternal / requestPermission paths can be exercised without building
// the iOS or Android apps. Production bundles never include this file
// (guarded by import.meta.env.DEV + dynamic import in main.ts).

type ResolveFn = (id: number, value: unknown) => void
type RejectFn = (id: number, err: string) => void

interface MockShuttleVPN {
  invoke: (msg: string) => void
}

interface ShuttleGlobals {
  ShuttleVPN?: MockShuttleVPN
  _shuttleResolve?: ResolveFn
  _shuttleReject?: RejectFn
}

export function mountMockBridge(): void {
  if (typeof window === 'undefined') return
  const w = window as unknown as ShuttleGlobals

  w.ShuttleVPN = {
    invoke(msg: string) {
      let parsed: { id?: number; action?: string; payload?: unknown }
      try { parsed = JSON.parse(msg) } catch { return }
      const { id, action, payload } = parsed
      if (typeof id !== 'number' || !action) return

      // Simulate a short native round-trip so the UI's "busy" / spinner
      // states are visible during testing.
      setTimeout(() => {
        const resolve = w._shuttleResolve
        const reject = w._shuttleReject
        if (!resolve || !reject) return

        switch (action) {
          case 'requestPermission':
            resolve(id, 'granted')
            break
          case 'scanQR':
            // Fake a plausible Shuttle URI for testing the AddSheet paste flow.
            resolve(id, 'shuttle://password@fake.example.com:443?name=Mock%20Server')
            break
          case 'share':
            console.info('[dev-bridge] share payload:', payload)
            resolve(id, 'ok')
            break
          case 'openExternal': {
            const url = (payload as { url?: string } | undefined)?.url
            if (url) console.info('[dev-bridge] openExternal:', url)
            resolve(id, 'ok')
            break
          }
          case 'subscribeStatus':
            reject(id, 'subscribeStatus not supported in mock')
            break
          default:
            reject(id, `unknown action: ${action}`)
        }
      }, 200)
    },
  }
  console.info('[dev-bridge] mounted. Shuttle native bridge is mocked.')
}

// Auto-mount on import if the URL opted in.
if (typeof window !== 'undefined' && window.location.search.includes('mockbridge=1')) {
  mountMockBridge()
}
