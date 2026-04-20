import { describe, it, expect, beforeEach } from 'vitest'
import {
  useSpeedHistory,
  __pushHistorySample,
  __resetHistory,
} from '@/features/dashboard/resource.svelte'

beforeEach(() => __resetHistory())

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
