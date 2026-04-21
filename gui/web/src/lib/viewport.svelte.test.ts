import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { viewport, __reset } from './viewport.svelte'

describe('viewport', () => {
  beforeEach(() => { __reset() })
  afterEach(() => { vi.restoreAllMocks() })

  it('classifies widths into form buckets', () => {
    __reset(360)
    expect(viewport.form).toBe('xs')
    expect(viewport.isMobile).toBe(true)
    expect(viewport.isTablet).toBe(false)
    expect(viewport.isDesktop).toBe(false)

    __reset(600)
    expect(viewport.form).toBe('sm')
    expect(viewport.isMobile).toBe(true)

    __reset(820)
    expect(viewport.form).toBe('md')
    expect(viewport.isMobile).toBe(false)
    expect(viewport.isTablet).toBe(true)

    __reset(1280)
    expect(viewport.form).toBe('lg')
    expect(viewport.isDesktop).toBe(true)

    __reset(1600)
    expect(viewport.form).toBe('xl')
  })

  it('detects touch via pointer: coarse media query', () => {
    const matchMedia = vi.fn((q: string) => ({
      matches: q === '(pointer: coarse)',
      media: q, onchange: null,
      addEventListener: () => {}, removeEventListener: () => {},
      addListener: () => {}, removeListener: () => {}, dispatchEvent: () => false,
    }))
    Object.defineProperty(window, 'matchMedia', { value: matchMedia, writable: true })
    __reset(500)
    expect(viewport.isTouch).toBe(true)
  })
})
