import { describe, it, expect } from 'vitest'
import { errorMessage } from './format'

describe('errorMessage', () => {
  it('returns Error.message', () => {
    expect(errorMessage(new Error('boom'))).toBe('boom')
  })
  it('returns strings verbatim', () => {
    expect(errorMessage('bridge offline')).toBe('bridge offline')
  })
  it('reads .message from plain objects', () => {
    expect(errorMessage({ message: 'rpc failed' })).toBe('rpc failed')
  })
  it('stringifies unknown shapes', () => {
    expect(errorMessage(42)).toBe('42')
    expect(errorMessage(null)).toBe('null')
  })
})
