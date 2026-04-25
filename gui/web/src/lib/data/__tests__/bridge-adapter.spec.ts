import { describe, it, expect, vi } from 'vitest'
import { BridgeAdapter } from '../bridge-adapter'
import { ApiError, TransportError } from '../types'

function fakeBridge(impl: (env: any) => Promise<any>) {
  ;(globalThis as any).window = { ShuttleBridge: { send: vi.fn(impl) } }
}

function ok(body: unknown, status = 200): any {
  return { status, headers: { 'content-type': 'application/json' }, body: btoa(JSON.stringify(body)) }
}

describe('BridgeAdapter.request', () => {
  it('parses 200 JSON', async () => {
    fakeBridge(async () => ok({ ok: 1 }))
    const a = new BridgeAdapter()
    expect(await a.request({ method: 'GET', path: '/api/x' })).toEqual({ ok: 1 })
  })

  it('throws ApiError on 4xx', async () => {
    fakeBridge(async () => ok({ error: 'gone' }, 404))
    const a = new BridgeAdapter()
    await expect(a.request({ method: 'GET', path: '/x' })).rejects.toBeInstanceOf(ApiError)
  })

  it('throws TransportError when status=-1', async () => {
    fakeBridge(async () => ({ status: -1, headers: {}, body: '', error: 'no response' }))
    const a = new BridgeAdapter()
    await expect(a.request({ method: 'GET', path: '/x' })).rejects.toBeInstanceOf(TransportError)
  })

  it('throws TransportError when bridge.send rejects', async () => {
    fakeBridge(async () => { throw new Error('IPC fail') })
    const a = new BridgeAdapter()
    await expect(a.request({ method: 'GET', path: '/x' })).rejects.toBeInstanceOf(TransportError)
  })

  it('encodes JSON body as base64', async () => {
    let captured: any
    fakeBridge(async (env) => { captured = env; return ok({}) })
    const a = new BridgeAdapter()
    await a.request({ method: 'POST', path: '/x', body: { a: 1 } })
    expect(captured.body).toBe(btoa(JSON.stringify({ a: 1 })))
  })
})
