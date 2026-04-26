import { describe, it, expect, vi, beforeEach } from 'vitest'
import { HttpAdapter } from '../http-adapter'
import { ApiError, TransportError } from '../types'

function mockFetch(impl: () => Promise<Response> | Response) {
  ;(globalThis as any).fetch = vi.fn(() => Promise.resolve(impl()))
}

describe('HttpAdapter.request', () => {
  beforeEach(() => { vi.useRealTimers() })

  it('parses 200 JSON', async () => {
    mockFetch(() => new Response(JSON.stringify({ ok: 1 }), { status: 200, headers: { 'content-type': 'application/json' } }))
    const a = new HttpAdapter()
    expect(await a.request({ method: 'GET', path: '/x' })).toEqual({ ok: 1 })
  })

  it('returns undefined for 204', async () => {
    mockFetch(() => new Response(null, { status: 204 }))
    const a = new HttpAdapter()
    expect(await a.request({ method: 'POST', path: '/x', body: {} })).toBeUndefined()
  })

  it('throws ApiError on 404 with server message', async () => {
    mockFetch(() => new Response(JSON.stringify({ error: 'gone', code: 'NOT_FOUND' }), { status: 404, headers: { 'content-type': 'application/json' } }))
    const a = new HttpAdapter()
    await expect(a.request({ method: 'GET', path: '/x' })).rejects.toBeInstanceOf(ApiError)
    try { await a.request({ method: 'GET', path: '/x' }) } catch (e: any) {
      expect(e.status).toBe(404)
      expect(e.code).toBe('NOT_FOUND')
      expect(e.message).toBe('gone')
    }
  })

  it('throws TransportError on network failure', async () => {
    mockFetch(() => Promise.reject(new TypeError('fetch failed')))
    const a = new HttpAdapter()
    await expect(a.request({ method: 'GET', path: '/x' })).rejects.toBeInstanceOf(TransportError)
  })

  it('honors AbortSignal', async () => {
    mockFetch(() => new Promise<Response>((_resolve, reject) => {
      setTimeout(() => reject(new DOMException('aborted', 'AbortError')), 10)
    }))
    const a = new HttpAdapter()
    const ctl = new AbortController()
    setTimeout(() => ctl.abort(), 1)
    await expect(a.request({ method: 'GET', path: '/x', signal: ctl.signal })).rejects.toBeDefined()
  })

  it('injects auth header from token getter', async () => {
    const fetchMock = vi.fn<typeof fetch>(async () => new Response('{}', { status: 200, headers: { 'content-type': 'application/json' } }))
    ;(globalThis as any).fetch = fetchMock
    const a = new HttpAdapter({ authToken: () => 'sekret' })
    await a.request({ method: 'GET', path: '/x' })
    const init = fetchMock.mock.calls[0][1]!
    expect((init.headers as any)['Authorization']).toBe('Bearer sekret')
  })

  it('preserves caller-supplied Authorization header (does not overwrite)', async () => {
    const fetchMock = vi.fn<typeof fetch>(async () => new Response('{}', { status: 200, headers: { 'content-type': 'application/json' } }))
    ;(globalThis as any).fetch = fetchMock
    const a = new HttpAdapter({ authToken: () => 'sekret' })
    await a.request({ method: 'GET', path: '/x', headers: { Authorization: 'Bearer caller-token' } })
    const init = fetchMock.mock.calls[0][1]!
    expect((init.headers as any)['Authorization']).toBe('Bearer caller-token')
  })
})

describe('HttpAdapter — diagnostics integration', () => {
  it('exposes a diagnostics instance', () => {
    const a = new HttpAdapter()
    expect(a.diagnostics).toBeDefined()
    expect(a.diagnostics.snapshot().requestsTotal).toBe(0)
  })

  it('records a successful request', async () => {
    mockFetch(() => new Response('{}', { status: 200, headers: { 'content-type': 'application/json' } }))
    const a = new HttpAdapter()
    await a.request({ method: 'GET', path: '/x' })
    const snap = a.diagnostics.snapshot()
    expect(snap.requestsTotal).toBe(1)
    expect(snap.requestsErr).toBe(0)
    expect(snap.lastError).toBeNull()
  })

  it('records an ApiError request with status in lastError', async () => {
    mockFetch(() => new Response(JSON.stringify({ error: 'gone' }), { status: 404, headers: { 'content-type': 'application/json' } }))
    const a = new HttpAdapter()
    await expect(a.request({ method: 'GET', path: '/x' })).rejects.toBeInstanceOf(ApiError)
    const snap = a.diagnostics.snapshot()
    expect(snap.requestsTotal).toBe(1)
    expect(snap.requestsErr).toBe(1)
    expect(snap.lastError!.reason).toMatch(/gone|404/)
  })

  it('records a TransportError request with cause message in lastError', async () => {
    mockFetch(() => Promise.reject(new TypeError('fetch failed')))
    const a = new HttpAdapter()
    await expect(a.request({ method: 'GET', path: '/x' })).rejects.toBeInstanceOf(TransportError)
    const snap = a.diagnostics.snapshot()
    expect(snap.requestsErr).toBe(1)
    expect(snap.lastError!.reason).toMatch(/fetch failed/)
  })

  it('user-initiated AbortError is counted as request, NOT error', async () => {
    mockFetch(() => new Promise<Response>((_resolve, reject) => {
      setTimeout(() => reject(new DOMException('aborted', 'AbortError')), 10)
    }))
    const a = new HttpAdapter()
    const ctl = new AbortController()
    setTimeout(() => ctl.abort(), 1)
    await expect(a.request({ method: 'GET', path: '/x', signal: ctl.signal })).rejects.toBeDefined()
    const snap = a.diagnostics.snapshot()
    expect(snap.requestsTotal).toBe(1)
    expect(snap.requestsErr).toBe(0)
    expect(snap.lastError).toBeNull()
  })

  it('internal timeout abort is recorded as error with reason "timeout"', async () => {
    mockFetch(() => new Promise<Response>((_resolve, reject) => {
      // Never resolves on its own — only the internal timeout will end this.
      setTimeout(() => reject(new DOMException('aborted', 'AbortError')), 100)
    }))
    const a = new HttpAdapter({ defaultTimeoutMs: 5 })
    await expect(a.request({ method: 'GET', path: '/x' })).rejects.toBeInstanceOf(TransportError)
    const snap = a.diagnostics.snapshot()
    expect(snap.requestsErr).toBe(1)
    expect(snap.lastError!.reason).toBe('timeout')
  })
})
