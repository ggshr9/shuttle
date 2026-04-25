import { describe, it, expect, vi, beforeEach } from 'vitest'
import { HttpSubscription } from '../http-subscription'
import { ConnectionStateController } from '../connection-state'

class FakeWebSocket {
  static instances: FakeWebSocket[] = []
  url: string
  readyState = 0
  onopen?: () => void
  onmessage?: (ev: { data: string }) => void
  onclose?: () => void
  onerror?: () => void
  closed = false
  constructor(url: string) {
    this.url = url
    FakeWebSocket.instances.push(this)
    queueMicrotask(() => { this.readyState = 1; this.onopen?.() })
  }
  send(_: string) {}
  close() { this.closed = true; this.readyState = 3; this.onclose?.() }
  emitMessage(payload: unknown) { this.onmessage?.({ data: JSON.stringify(payload) }) }
}

describe('HttpSubscription', () => {
  let conn: ConnectionStateController

  beforeEach(() => {
    FakeWebSocket.instances = []
    conn = new ConnectionStateController()
    ;(globalThis as any).WebSocket = FakeWebSocket
  })

  it('opens one WS for any number of subscribers (snapshot)', () => {
    const sub = new HttpSubscription<{ a: number }>('status', 'snapshot', '/ws/status', conn)
    sub.add(() => {})
    sub.add(() => {})
    expect(FakeWebSocket.instances.length).toBe(1)
  })

  it('closes WS when last subscriber leaves', async () => {
    const sub = new HttpSubscription<{ a: number }>('status', 'snapshot', '/ws/status', conn)
    const off1 = sub.add(() => {})
    const off2 = sub.add(() => {})
    off1(); off2()
    expect(FakeWebSocket.instances[0].closed).toBe(true)
  })

  it('emits messages to subscribers', async () => {
    const sub = new HttpSubscription<{ v: number }>('status', 'snapshot', '/ws/status', conn)
    const cb = vi.fn()
    sub.add(cb)
    await Promise.resolve()
    FakeWebSocket.instances[0].emitMessage({ v: 7 })
    expect(cb).toHaveBeenCalledWith({ v: 7 })
  })

  it('reports ok to ConnectionStateController on first message', async () => {
    const sub = new HttpSubscription<{ v: number }>('status', 'snapshot', '/ws/status', conn)
    sub.add(() => {})
    await Promise.resolve()
    FakeWebSocket.instances[0].emitMessage({ v: 1 })
    expect(conn.value).toBe('connected')
  })
})
