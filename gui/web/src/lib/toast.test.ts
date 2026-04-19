import { describe, it, expect, beforeEach } from 'vitest'
import { toasts, dismissAll } from '@/lib/toaster.svelte'

beforeEach(() => dismissAll())

describe('toasts', () => {
  it('success() adds an item', () => {
    toasts.success('hi', 0)   // duration 0 disables auto-dismiss
    expect(toasts.items.length).toBe(1)
    expect(toasts.items[0].type).toBe('success')
    expect(toasts.items[0].message).toBe('hi')
  })

  it('error() uses error type', () => {
    toasts.error('oops', 0)
    expect(toasts.items[0].type).toBe('error')
  })

  it('dismiss removes by id', () => {
    const id = toasts.info('x', 0)
    expect(toasts.items.length).toBe(1)
    toasts.dismiss(id)
    expect(toasts.items.length).toBe(0)
  })

  it('dismissAll clears', () => {
    toasts.info('a', 0)
    toasts.warning('b', 0)
    dismissAll()
    expect(toasts.items.length).toBe(0)
  })
})
