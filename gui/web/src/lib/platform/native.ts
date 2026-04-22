import type { Platform, PlatformName, CapResult, SharePayload } from './types'
import type { Status } from '../api/types'
import { connect, disconnect, status as getStatus } from '../api/endpoints'
import { callBridge } from './shuttle-bridge'

interface ShuttleBridge {
  invoke?: (msg: string) => void
  start?: () => Promise<void> | void
  stop?: () => Promise<void> | void
  isRunning?: () => Promise<boolean> | boolean
  requestPermission?: () => Promise<'granted' | 'denied'>
  scanQR?: () => Promise<string>
  share?: (payload: SharePayload) => Promise<'ok' | 'cancelled'>
  openExternal?: (url: string) => Promise<void> | void
  subscribeStatus?: (cb: (s: Status) => void) => () => void
}

function bridge(): ShuttleBridge | null {
  if (typeof window === 'undefined') return null
  return (window as unknown as { ShuttleVPN?: ShuttleBridge }).ShuttleVPN ?? null
}

function hasMethod<K extends keyof ShuttleBridge>(k: K): boolean {
  const b = bridge()
  return !!b && typeof b[k] === 'function'
}

/**
 * True when the bridge exposes *any* way to dispatch `action` — either via
 * the unified `invoke(jsonMsg)` function (Phase 4 style) or as a direct
 * per-method function (legacy Phase 1 style). callBridge() handles both
 * styles internally; we check capability first to produce `'unsupported'`
 * without triggering a failing Promise.
 */
function canDispatch(action: keyof ShuttleBridge): boolean {
  const b = bridge()
  if (!b) return false
  return typeof b.invoke === 'function' || typeof b[action] === 'function'
}

export class NativePlatform implements Platform {
  readonly name: PlatformName = 'native'

  async engineStart(): Promise<void> {
    if (hasMethod('start')) {
      await bridge()!.start!()
      return
    }
    await connect()
  }

  async engineStop(): Promise<void> {
    if (hasMethod('stop')) {
      await bridge()!.stop!()
      return
    }
    await disconnect()
  }

  async engineStatus(): Promise<Status> { return getStatus() }

  async requestVpnPermission(): Promise<CapResult<'granted' | 'denied'>> {
    if (!canDispatch('requestPermission')) return 'unsupported'
    return await callBridge<'granted' | 'denied'>('requestPermission')
  }

  async scanQRCode(): Promise<CapResult<string>> {
    if (!canDispatch('scanQR')) return 'unsupported'
    return await callBridge<string>('scanQR')
  }

  async share(payload: SharePayload): Promise<CapResult<'ok' | 'cancelled'>> {
    if (!canDispatch('share')) return 'unsupported'
    return await callBridge<'ok' | 'cancelled'>('share', payload)
  }

  async openExternalUrl(url: string): Promise<CapResult<'ok'>> {
    if (!canDispatch('openExternal')) return 'unsupported'
    await callBridge<void>('openExternal', { url })
    return 'ok'
  }

  onStatusChange(cb: (s: Status) => void): () => void {
    if (hasMethod('subscribeStatus')) {
      return bridge()!.subscribeStatus!(cb)
    }
    const timer = setInterval(async () => {
      try { cb(await this.engineStatus()) } catch {}
    }, 2000)
    return () => clearInterval(timer)
  }
}
