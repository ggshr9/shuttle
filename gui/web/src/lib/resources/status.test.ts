import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setAdapter, __resetAdapter } from '@/lib/data'
import { HttpAdapter } from '@/lib/data/http-adapter'
import {
  useSpeedHistory,
  __pushHistorySample,
  __resetHistory,
  __resetSpeedStream,
  __resetHistoryInitialized,
} from '@/lib/resources/status.svelte'

// Minimal WebSocket stub — HttpAdapter opens a WS for each topic but the
// speed-history tests never push real data through it.
class FakeWebSocket {
  static instances: FakeWebSocket[] = []
  readyState = 0
  onopen?: () => void
  onmessage?: (ev: { data: string }) => void
  onclose?: () => void
  onerror?: () => void
  constructor(public url: string) { FakeWebSocket.instances.push(this) }
  send(_: string) {}
  close() { this.readyState = 3; this.onclose?.() }
}

beforeEach(() => {
  FakeWebSocket.instances = []
  ;(globalThis as any).WebSocket = FakeWebSocket
  __resetAdapter()
  __resetSpeedStream()
  __resetHistoryInitialized()
  __resetHistory()
  setAdapter(new HttpAdapter())
})

describe('useSpeedHistory', () => {
  it('starts empty', () => {
    const h = useSpeedHistory()
    expect(h.up.length).toBe(0)
    expect(h.down.length).toBe(0)
  })

  it('appends samples in order', () => {
    __pushHistorySample({ upload: 1, download: 10 })
    __pushHistorySample({ upload: 2, download: 20 })
    const h = useSpeedHistory()
    expect(h.up).toEqual([1, 2])
    expect(h.down).toEqual([10, 20])
  })

  it('caps at 60 samples', () => {
    for (let i = 0; i < 70; i++) __pushHistorySample({ upload: i, download: i * 10 })
    const h = useSpeedHistory()
    expect(h.up.length).toBe(60)
    expect(h.up[0]).toBe(10)
    expect(h.up[59]).toBe(69)
  })
})
