import { describe, it, expect } from 'vitest'
import { ApiError, TransportError } from '../types'

describe('error types', () => {
  it('ApiError carries status and code', () => {
    const e = new ApiError(404, 'NOT_FOUND', 'server not found')
    expect(e).toBeInstanceOf(Error)
    expect(e.status).toBe(404)
    expect(e.code).toBe('NOT_FOUND')
    expect(e.message).toBe('server not found')
  })

  it('TransportError carries cause', () => {
    const cause = new Error('connection refused')
    const e = new TransportError(cause, 'IPC failed')
    expect(e).toBeInstanceOf(Error)
    expect(e.cause).toBe(cause)
    expect(e.message).toBe('IPC failed')
  })

  it('errors are distinguishable via instanceof', () => {
    const a = new ApiError(500, undefined, 'oops')
    const t = new TransportError(null, 'oops')
    expect(a instanceof ApiError).toBe(true)
    expect(a instanceof TransportError).toBe(false)
    expect(t instanceof TransportError).toBe(true)
    expect(t instanceof ApiError).toBe(false)
  })
})
