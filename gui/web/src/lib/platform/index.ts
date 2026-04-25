import type { Platform, PlatformName } from './types'
import { WebPlatform } from './web'
import { NativePlatform } from './native'
import { WailsPlatform } from './wails'

export type { Platform, PlatformName, CapResult, SharePayload } from './types'

export function detect(): PlatformName {
  if (typeof window === 'undefined') return 'web'
  if ((window as any).go?.main?.App) return 'wails'
  // iOS VPN mode: data IPC bridge (Phase 5 — envelope send to extension).
  // iOS proxy mode: legacy capability bridge (engineStart, scanQR, share, etc.).
  // Both map to platform name 'native'; they may co-exist in VPN mode.
  if ((window as any).ShuttleBridge) return 'native'
  if ((window as any).ShuttleVPN) return 'native'
  return 'web'
}

let _instance: Platform | null = null

export function getPlatform(): Platform {
  if (_instance) return _instance
  const name = detect()
  _instance = name === 'wails'  ? new WailsPlatform()
            : name === 'native' ? new NativePlatform()
            :                     new WebPlatform()
  return _instance
}

// Convenience: `platform` accessor — always returns the singleton.
// Binds functions to the instance so `this` resolves correctly even when
// consumers destructure or rebind (e.g. `const { engineStart } = platform`).
export const platform = new Proxy({} as Platform, {
  get(_t, prop) {
    const instance = getPlatform()
    const value = (instance as any)[prop]
    return typeof value === 'function' ? value.bind(instance) : value
  },
})

// Test helper
export function __resetPlatform(): void { _instance = null }
