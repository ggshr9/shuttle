// gui/web/src/lib/data/__tests__/subscription-base.spec.ts
import { describe, it, expect, vi } from 'vitest'
import { SubscriptionBase } from '../subscription-base'

class TestSub<T> extends SubscriptionBase<T> {
  connectCount = 0
  disconnectCount = 0
  protected connect(): void { this.connectCount++ }
  protected disconnect(): void { this.disconnectCount++ }
  protected async tick(): Promise<void> { /* no-op */ }
  // expose protected emit for direct test driving
  pushValue(v: T) { this.emit(v) }
}

describe('SubscriptionBase ref counting', () => {
  it('connects on first subscriber', () => {
    const s = new TestSub<number>('status', 'snapshot')
    s.add(() => {})
    expect(s.connectCount).toBe(1)
  })

  it('does not reconnect for subsequent subscribers', () => {
    const s = new TestSub<number>('status', 'snapshot')
    s.add(() => {})
    s.add(() => {})
    expect(s.connectCount).toBe(1)
  })

  it('disconnects when last subscriber leaves', () => {
    const s = new TestSub<number>('status', 'snapshot')
    const off1 = s.add(() => {})
    const off2 = s.add(() => {})
    off1()
    expect(s.disconnectCount).toBe(0)
    off2()
    expect(s.disconnectCount).toBe(1)
  })

  it('snapshot replay: late subscriber gets cached value', async () => {
    const s = new TestSub<number>('status', 'snapshot')
    s.add(() => {})
    s.pushValue(42)
    const cb = vi.fn()
    s.add(cb)
    await Promise.resolve()  // queueMicrotask flush
    expect(cb).toHaveBeenCalledWith(42)
  })

  it('stream replay: late subscriber does NOT get cached value', async () => {
    const s = new TestSub<number>('logs', 'stream')
    s.add(() => {})
    s.pushValue(42)
    const cb = vi.fn()
    s.add(cb)
    await Promise.resolve()
    expect(cb).not.toHaveBeenCalled()
  })
})
