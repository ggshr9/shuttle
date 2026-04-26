import { describe, it, expect, beforeEach } from 'vitest'
import { bridgeSend } from '../bridge-transport'

describe('bridgeSend', () => {
  let posted: Array<{ id: number; envelope: any }> = []
  let bridge: any

  beforeEach(() => {
    posted = []
    // Fake the postMessage / _complete handshake.
    bridge = {
      send(envelope: any) {
        const id = ++bridge._counter
        return new Promise((resolve, reject) => {
          bridge._pending.set(id, { resolve, reject })
          posted.push({ id, envelope })
        })
      },
      _complete(id: number, response: any) {
        const p = bridge._pending.get(id)
        if (p) { bridge._pending.delete(id); p.resolve(response) }
      },
      _fail(id: number, msg: string) {
        const p = bridge._pending.get(id)
        if (p) { bridge._pending.delete(id); p.reject(new Error(msg)) }
      },
      _counter: 0,
      _pending: new Map<number, any>(),
    }
    ;(globalThis as any).window = { ShuttleBridge: bridge }
  })

  it('forwards request envelopes', async () => {
    const p = bridgeSend({ method: 'GET', path: '/api/x', headers: {} })
    expect(posted.length).toBe(1)
    expect(posted[0].envelope.path).toBe('/api/x')
    bridge._complete(posted[0].id, { status: 200, headers: {}, body: btoa('{}'), error: null })
    const res = await p
    expect(res.status).toBe(200)
  })

  it('rejects on _fail', async () => {
    const p = bridgeSend({ method: 'GET', path: '/x', headers: {} })
    bridge._fail(posted[0].id, 'boom')
    await expect(p).rejects.toThrow('boom')
  })

  it('throws if window.ShuttleBridge missing', () => {
    delete (globalThis as any).window.ShuttleBridge
    expect(() => bridgeSend({ method: 'GET', path: '/x', headers: {} })).toThrow(/ShuttleBridge/)
  })
})
