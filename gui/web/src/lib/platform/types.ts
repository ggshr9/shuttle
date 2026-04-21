import type { Status } from '../api/types'

export type PlatformName = 'web' | 'native' | 'wails'

export type CapResult<T> = T | 'unsupported'

export interface SharePayload {
  title?: string
  text?: string
  url?: string
}

export interface Platform {
  readonly name: PlatformName

  // Engine lifecycle
  engineStart(): Promise<void>
  engineStop(): Promise<void>
  engineStatus(): Promise<Status>

  // OS-level capabilities
  requestVpnPermission(): Promise<CapResult<'granted' | 'denied'>>
  scanQRCode(): Promise<CapResult<string>>
  share(payload: SharePayload): Promise<CapResult<'ok' | 'cancelled'>>
  openExternalUrl(url: string): Promise<CapResult<'ok'>>

  // Observation
  onStatusChange(cb: (s: Status) => void): () => void
}
