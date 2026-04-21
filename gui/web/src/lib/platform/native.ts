import type { Platform, PlatformName, CapResult, SharePayload } from './types'
import type { Status } from '../api/types'
import { connect, disconnect, status as getStatus } from '../api/endpoints'

interface ShuttleBridge {
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
  return (window as any).ShuttleVPN ?? null
}

function hasMethod<K extends keyof ShuttleBridge>(k: K): boolean {
  const b = bridge()
  return !!b && typeof b[k] === 'function'
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
    if (!hasMethod('requestPermission')) return 'unsupported'
    return await bridge()!.requestPermission!()
  }

  async scanQRCode(): Promise<CapResult<string>> {
    if (!hasMethod('scanQR')) return 'unsupported'
    return await bridge()!.scanQR!()
  }

  async share(payload: SharePayload): Promise<CapResult<'ok' | 'cancelled'>> {
    if (!hasMethod('share')) return 'unsupported'
    return await bridge()!.share!(payload)
  }

  async openExternalUrl(url: string): Promise<CapResult<'ok'>> {
    if (!hasMethod('openExternal')) return 'unsupported'
    await bridge()!.openExternal!(url)
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
