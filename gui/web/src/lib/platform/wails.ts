import type { Platform, PlatformName, CapResult, SharePayload } from './types'
import type { Status } from '../api/types'
import { connect, disconnect, status as getStatus } from '../api/endpoints'

function wailsApp(): Record<string, any> | null {
  if (typeof window === 'undefined') return null
  return ((window as any).go?.main?.App) ?? null
}

function wailsRuntime(): Record<string, any> | null {
  if (typeof window === 'undefined') return null
  return (window as any).runtime ?? null
}

export class WailsPlatform implements Platform {
  readonly name: PlatformName = 'wails'

  async engineStart(): Promise<void> {
    const app = wailsApp()
    if (app?.EngineStart) { await app.EngineStart(); return }
    await connect()
  }
  async engineStop(): Promise<void> {
    const app = wailsApp()
    if (app?.EngineStop) { await app.EngineStop(); return }
    await disconnect()
  }
  async engineStatus(): Promise<Status> {
    const app = wailsApp()
    if (app?.EngineStatus) return await app.EngineStatus()
    return getStatus()
  }

  async requestVpnPermission(): Promise<CapResult<'granted' | 'denied'>> { return 'unsupported' }
  async scanQRCode(): Promise<CapResult<string>> { return 'unsupported' }
  async share(): Promise<CapResult<'ok' | 'cancelled'>> { return 'unsupported' }

  async openExternalUrl(url: string): Promise<CapResult<'ok'>> {
    const r = wailsRuntime()
    if (r?.BrowserOpenURL) { r.BrowserOpenURL(url); return 'ok' }
    return 'unsupported'
  }

  onStatusChange(cb: (s: Status) => void): () => void {
    const timer = setInterval(async () => {
      try { cb(await this.engineStatus()) } catch {}
    }, 2000)
    return () => clearInterval(timer)
  }
}
