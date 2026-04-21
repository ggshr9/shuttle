import { describe, it, expect, vi, beforeEach } from 'vitest'
import { WebPlatform } from './web'
import * as endpoints from '../api/endpoints'

describe('WebPlatform', () => {
  let p: WebPlatform
  beforeEach(() => {
    p = new WebPlatform()
    vi.restoreAllMocks()
  })

  it('engineStart posts /api/connect', async () => {
    const spy = vi.spyOn(endpoints, 'connect').mockResolvedValue(undefined as any)
    await p.engineStart()
    expect(spy).toHaveBeenCalled()
  })

  it('requestVpnPermission returns unsupported on web', async () => {
    expect(await p.requestVpnPermission()).toBe('unsupported')
  })

  it('scanQRCode returns unsupported on web', async () => {
    expect(await p.scanQRCode()).toBe('unsupported')
  })

  it('share uses navigator.share when available', async () => {
    const share = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'share', { value: share, configurable: true })
    expect(await p.share({ title: 't', url: 'u' })).toBe('ok')
    expect(share).toHaveBeenCalledWith({ title: 't', url: 'u' })
  })

  it('share falls back to clipboard when navigator.share missing', async () => {
    Object.defineProperty(navigator, 'share', { value: undefined, configurable: true })
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    expect(await p.share({ url: 'https://x' })).toBe('ok')
    expect(writeText).toHaveBeenCalledWith('https://x')
  })

  it('openExternalUrl calls window.open', async () => {
    const open = vi.fn()
    vi.stubGlobal('open', open)
    expect(await p.openExternalUrl('https://x')).toBe('ok')
    expect(open).toHaveBeenCalledWith('https://x', '_blank')
  })

  it('name === web', () => { expect(p.name).toBe('web') })
})
