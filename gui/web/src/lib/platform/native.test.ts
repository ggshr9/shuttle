import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { NativePlatform } from './native'

function mountBridge(methods: Record<string, any>) {
  (window as any).ShuttleVPN = methods
}

describe('NativePlatform', () => {
  let p: NativePlatform
  beforeEach(() => {
    p = new NativePlatform()
    delete (window as any).ShuttleVPN
  })
  afterEach(() => { vi.restoreAllMocks() })

  it('requestVpnPermission returns unsupported when method missing', async () => {
    mountBridge({})
    expect(await p.requestVpnPermission()).toBe('unsupported')
  })

  it('requestVpnPermission calls bridge when present', async () => {
    const fn = vi.fn().mockResolvedValue('granted')
    mountBridge({ requestPermission: fn })
    expect(await p.requestVpnPermission()).toBe('granted')
    expect(fn).toHaveBeenCalled()
  })

  it('scanQRCode returns unsupported when method missing', async () => {
    mountBridge({})
    expect(await p.scanQRCode()).toBe('unsupported')
  })

  it('scanQRCode returns scanned string when bridge present', async () => {
    mountBridge({ scanQR: vi.fn().mockResolvedValue('shuttle://abc') })
    expect(await p.scanQRCode()).toBe('shuttle://abc')
  })

  it('name === native', () => { expect(p.name).toBe('native') })
})
