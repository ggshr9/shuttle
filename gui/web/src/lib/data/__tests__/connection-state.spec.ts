import { describe, it, expect } from 'vitest'
import { ConnectionStateController } from '../connection-state'

describe('ConnectionStateController', () => {
  it('starts idle when no topics reported', () => {
    const c = new ConnectionStateController()
    expect(c.value).toBe('idle')
  })

  it('flips to connected when any topic reports ok', () => {
    const c = new ConnectionStateController()
    c.report('status', 'ok')
    expect(c.value).toBe('connected')
  })

  it('flips to error when all topics report error', () => {
    const c = new ConnectionStateController()
    c.report('status', 'error')
    c.report('logs', 'error')
    expect(c.value).toBe('error')
  })

  it('stays connected if at least one topic is ok', () => {
    const c = new ConnectionStateController()
    c.report('status', 'ok')
    c.report('logs', 'error')
    expect(c.value).toBe('connected')
  })

  it('clear() removes a topic', () => {
    const c = new ConnectionStateController()
    c.report('status', 'ok')
    c.clear('status')
    expect(c.value).toBe('idle')
  })

  it('subscribers receive updates', () => {
    const c = new ConnectionStateController()
    const seen: string[] = []
    c.subscribe(v => seen.push(v))
    c.report('status', 'ok')
    c.report('status', 'error')
    expect(seen).toEqual(['idle', 'connected', 'error'])
  })

  it('unsubscribe stops further notifications', () => {
    const c = new ConnectionStateController()
    const seen: string[] = []
    const off = c.subscribe(v => seen.push(v))
    c.report('status', 'ok')
    off()
    c.report('status', 'error')
    expect(seen).toEqual(['idle', 'connected'])
  })
})
