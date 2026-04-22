import { describe, it, expect, vi, beforeEach } from 'vitest'
import { callBridge } from './shuttle-bridge'

type Bridge = {
  invoke?: (msg: string) => void
  [method: string]: unknown
}

function mountBridge(impl: Bridge) {
  (window as any).ShuttleVPN = impl
}

describe('callBridge', () => {
  beforeEach(() => {
    delete (window as any).ShuttleVPN
    delete (window as any)._shuttleResolve
    delete (window as any)._shuttleReject
    // Re-import side effects to rebind the global handlers.
    // vi.resetModules would normally do this; we rely on the module
    // initializing at first import.
  })

  it('rejects when bridge is missing', async () => {
    await expect(callBridge('requestPermission')).rejects.toThrow(/bridge not available/i)
  })

  it('dispatches via window.ShuttleVPN.invoke when present', async () => {
    const invoke = vi.fn()
    mountBridge({ invoke })
    // Fire the call but don't await — we resolve manually
    const p = callBridge<'granted'>('requestPermission')
    // The invoke mock was called with a JSON string containing an id
    expect(invoke).toHaveBeenCalledTimes(1)
    const msg = JSON.parse(invoke.mock.calls[0][0] as string)
    expect(msg.action).toBe('requestPermission')
    expect(typeof msg.id).toBe('number')
    // Resolve the pending promise
    ;(window as any)._shuttleResolve(msg.id, 'granted')
    await expect(p).resolves.toBe('granted')
  })

  it('rejects the promise when native reports an error', async () => {
    const invoke = vi.fn()
    mountBridge({ invoke })
    const p = callBridge('scanQR')
    const msg = JSON.parse(invoke.mock.calls[0][0] as string)
    ;(window as any)._shuttleReject(msg.id, 'user cancelled')
    await expect(p).rejects.toThrow('user cancelled')
  })

  it('falls back to direct method call when no invoke but method exists', async () => {
    const scanQR = vi.fn().mockResolvedValue('shuttle://xyz')
    mountBridge({ scanQR })
    const result = await callBridge<string>('scanQR')
    expect(scanQR).toHaveBeenCalled()
    expect(result).toBe('shuttle://xyz')
  })

  it('rejects when neither invoke nor the named method is present', async () => {
    mountBridge({})
    await expect(callBridge('scanQR')).rejects.toThrow(/not available/i)
  })

  it('passes payload through to invoke JSON', async () => {
    const invoke = vi.fn()
    mountBridge({ invoke })
    const p = callBridge('share', { title: 'Logs', text: 'abc' })
    const msg = JSON.parse(invoke.mock.calls[0][0] as string)
    expect(msg.payload).toEqual({ title: 'Logs', text: 'abc' })
    ;(window as any)._shuttleResolve(msg.id, 'ok')
    await p
  })
})
