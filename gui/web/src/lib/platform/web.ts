import type { Platform, PlatformName, CapResult, SharePayload } from './types'
import type { Status } from '../api/types'
import { connect, disconnect, status as getStatus } from '../api/endpoints'

export class WebPlatform implements Platform {
  readonly name: PlatformName = 'web'

  async engineStart(): Promise<void> { await connect() }
  async engineStop(): Promise<void> { await disconnect() }
  async engineStatus(): Promise<Status> { return getStatus() }

  async requestVpnPermission(): Promise<CapResult<'granted' | 'denied'>> {
    return 'unsupported'
  }
  async scanQRCode(): Promise<CapResult<string>> { return 'unsupported' }

  async share(payload: SharePayload): Promise<CapResult<'ok' | 'cancelled'>> {
    if (typeof navigator !== 'undefined' && typeof navigator.share === 'function') {
      try {
        await navigator.share(payload)
        return 'ok'
      } catch (e) {
        if ((e as Error).name === 'AbortError') return 'cancelled'
        // fall through to clipboard
      }
    }
    const text = payload.url ?? payload.text ?? payload.title ?? ''
    if (text && typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return 'ok'
    }
    return 'unsupported'
  }

  async openExternalUrl(url: string): Promise<CapResult<'ok'>> {
    if (typeof window !== 'undefined') {
      window.open(url, '_blank')
      return 'ok'
    }
    return 'unsupported'
  }

  onStatusChange(cb: (s: Status) => void): () => void {
    // WebSocket subscription reuses existing /api/events. Kept simple for now:
    // poll every 2s; upgrade to WS in Task 1.6.
    const timer = setInterval(async () => {
      try { cb(await this.engineStatus()) } catch {}
    }, 2000)
    return () => clearInterval(timer)
  }
}
