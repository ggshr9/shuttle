import { describe, it, expect, vi, beforeEach } from 'vitest'
import { WailsPlatform } from './wails'

function mountWails(api: Record<string, any>, runtime: Record<string, any> = {}) {
  (window as any).go = { main: { App: api } }
  ;(window as any).runtime = runtime
}

describe('WailsPlatform', () => {
  let p: WailsPlatform
  beforeEach(() => {
    delete (window as any).go
    delete (window as any).runtime
    p = new WailsPlatform()
  })

  it('engineStart calls Go binding when present', async () => {
    const fn = vi.fn().mockResolvedValue(undefined)
    mountWails({ EngineStart: fn })
    await p.engineStart()
    expect(fn).toHaveBeenCalled()
  })

  it('openExternalUrl uses Wails runtime BrowserOpenURL', async () => {
    const openURL = vi.fn()
    mountWails({}, { BrowserOpenURL: openURL })
    expect(await p.openExternalUrl('https://x')).toBe('ok')
    expect(openURL).toHaveBeenCalledWith('https://x')
  })

  it('scanQRCode returns unsupported on wails', async () => {
    mountWails({})
    expect(await p.scanQRCode()).toBe('unsupported')
  })

  it('name === wails', () => { expect(p.name).toBe('wails') })
})
