import type { Platform, PlatformName } from './types'
import { WebPlatform } from './web'
import { NativePlatform } from './native'
import { WailsPlatform } from './wails'

export type { Platform, PlatformName, CapResult, SharePayload } from './types'

export function detect(): PlatformName {
  if (typeof window === 'undefined') return 'web'
  if ((window as any).go?.main?.App) return 'wails'
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
export const platform = new Proxy({} as Platform, {
  get(_t, prop) { return (getPlatform() as any)[prop] },
})

// Test helper
export function __resetPlatform(): void { _instance = null }
