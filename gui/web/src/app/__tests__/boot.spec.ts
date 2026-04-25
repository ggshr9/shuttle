import { describe, it, expect, beforeEach, vi } from 'vitest'
import { boot } from '../boot'
import { __resetAdapter, getAdapter } from '@/lib/data'

describe('boot', () => {
  beforeEach(() => {
    __resetAdapter()
    delete (window as any).ShuttleBridge
    delete (window as any).webkit
    // Clear ?bridge query param mock
    Object.defineProperty(window, 'location', {
      value: { search: '' },
      writable: true,
    })
  })

  it('installs HttpAdapter when no bridge present', async () => {
    await boot()
    expect(getAdapter()).toBeDefined()
    expect(getAdapter().connectionState.value).toBe('idle')
  })

  it('installs HttpAdapter when ?bridge=0 forces it even with bridge present', async () => {
    Object.defineProperty(window, 'location', { value: { search: '?bridge=0' }, writable: true })
    ;(window as any).ShuttleBridge = { send: vi.fn() }
    await boot()
    expect(getAdapter().constructor.name).toBe('HttpAdapter')
  })

  it('requests fallback when bridge probe fails', async () => {
    const post = vi.fn()
    ;(window as any).webkit = { messageHandlers: { fallback: { postMessage: post } } }
    ;(window as any).ShuttleBridge = {
      send: async () => { throw new Error('unreachable') },
    }
    await boot()
    expect(post).toHaveBeenCalled()
    expect(post.mock.calls[0][0]).toMatchObject({ reason: expect.any(String) })
  })

  it('installs BridgeAdapter when probe succeeds', async () => {
    ;(window as any).ShuttleBridge = {
      send: async () => ({
        status: 200,
        headers: { 'content-type': 'application/json' },
        body: btoa(JSON.stringify({ status: 'ok' })),
        error: null,
      }),
    }
    await boot()
    expect(getAdapter().constructor.name).toBe('BridgeAdapter')
  })

  it('?bridge=1 force-installs BridgeAdapter even when probe fails', async () => {
    Object.defineProperty(window, 'location', { value: { search: '?bridge=1' }, writable: true })
    const post = vi.fn()
    ;(window as any).webkit = { messageHandlers: { fallback: { postMessage: post } } }
    ;(window as any).ShuttleBridge = {
      send: async () => { throw new Error('healthz unreachable') },
    }
    await boot()
    expect(post).not.toHaveBeenCalled()    // fallback NOT requested under force flag
    expect(getAdapter().constructor.name).toBe('BridgeAdapter')
  })
})

describe('boot — fallback diagnostics recording', () => {
  beforeEach(() => {
    __resetAdapter()
    delete (window as any).ShuttleBridge
    delete (window as any).webkit
    Object.defineProperty(window, 'location', { value: { search: '' }, writable: true })
    localStorage.clear()
  })

  it('persists fallback to localStorage before postMessage when probe fails', async () => {
    const post = vi.fn()
    ;(window as any).webkit = { messageHandlers: { fallback: { postMessage: post } } }
    ;(window as any).ShuttleBridge = { send: async () => { throw new Error('unreachable') } }

    let storedAtPostTime: string | null = null
    post.mockImplementation(() => {
      storedAtPostTime = localStorage.getItem('shuttle.diag.fallbacks')
    })

    await boot()
    expect(storedAtPostTime).toBeTruthy()
    const parsed = JSON.parse(storedAtPostTime!)
    expect(parsed.entries.length).toBeGreaterThan(0)
    expect(parsed.entries[0].reason).toMatch(/unreachable/i)
  })

  it('writes via persistDirect when ShuttleBridge is missing under bridge=1', async () => {
    Object.defineProperty(window, 'location', { value: { search: '?bridge=1' }, writable: true })
    const post = vi.fn()
    ;(window as any).webkit = { messageHandlers: { fallback: { postMessage: post } } }

    let storedAtPostTime: string | null = null
    post.mockImplementation(() => {
      storedAtPostTime = localStorage.getItem('shuttle.diag.fallbacks')
    })

    await boot()
    expect(post).toHaveBeenCalled()
    expect(storedAtPostTime).toBeTruthy()
    const parsed = JSON.parse(storedAtPostTime!)
    expect(parsed.total).toBeGreaterThanOrEqual(1)
  })

  it('does not register unhandledrejection listener when probe fails', async () => {
    const post = vi.fn()
    ;(window as any).webkit = { messageHandlers: { fallback: { postMessage: post } } }
    ;(window as any).ShuttleBridge = { send: async () => { throw new Error('dead') } }
    const addSpy = vi.spyOn(window, 'addEventListener')

    await boot()

    const listenerCalls = addSpy.mock.calls.filter(c => c[0] === 'unhandledrejection')
    expect(listenerCalls).toHaveLength(0)
    addSpy.mockRestore()
  })
})
