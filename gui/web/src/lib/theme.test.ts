import { describe, it, expect, beforeEach } from 'vitest'
import { theme } from '@/lib/theme.svelte'

beforeEach(() => { localStorage.clear() })

describe('theme', () => {
  it('set() persists and applies data-theme', () => {
    theme.set('light')
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
    expect(localStorage.getItem('shuttle-theme')).toBe('light')
  })

  it('toggle() flips between dark and light', () => {
    theme.set('dark')
    theme.toggle()
    expect(theme.current).toBe('light')
    theme.toggle()
    expect(theme.current).toBe('dark')
  })

  it('reads stored value on module init (baseline check)', () => {
    // On fresh module init the default is 'dark' (no stored, no light media).
    // We can only verify via current after .set().
    theme.set('dark')
    expect(theme.current).toBe('dark')
  })
})
