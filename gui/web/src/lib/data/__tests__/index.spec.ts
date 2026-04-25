import { describe, it, expect, beforeEach, vi } from 'vitest'
import { __resetAdapter, getAdapter, setAdapter } from '../index'

describe('adapter selection', () => {
  beforeEach(() => { __resetAdapter() })

  it('throws when not initialised', () => {
    expect(() => getAdapter()).toThrow(/not initialised/)
  })

  it('returns the registered adapter', () => {
    const fake: any = { request: vi.fn(), subscribe: vi.fn(), connectionState: { value: 'idle', subscribe: () => () => {} } }
    setAdapter(fake)
    expect(getAdapter()).toBe(fake)
  })

  it('setAdapter is idempotent — second call replaces', () => {
    const a: any = { _id: 'a', request: vi.fn(), subscribe: vi.fn(), connectionState: { value: 'idle', subscribe: () => () => {} } }
    const b: any = { _id: 'b', request: vi.fn(), subscribe: vi.fn(), connectionState: { value: 'idle', subscribe: () => () => {} } }
    setAdapter(a)
    setAdapter(b)
    expect(getAdapter()).toBe(b)
  })
})
