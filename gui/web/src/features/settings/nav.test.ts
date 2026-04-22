import { describe, it, expect } from 'vitest'
import { subNav, sectionOrder, entriesBySection, DEFAULT_SLUG } from './nav'

describe('settings nav', () => {
  it('exports DEFAULT_SLUG pointing at a real entry', () => {
    expect(subNav.some((e) => e.slug === DEFAULT_SLUG)).toBe(true)
  })

  it('every entry belongs to exactly one listed section', () => {
    for (const entry of subNav) {
      expect(sectionOrder).toContain(entry.section)
    }
  })

  it('no duplicate slugs across the subnav', () => {
    const slugs = subNav.map((e) => e.slug)
    expect(new Set(slugs).size).toBe(slugs.length)
  })

  it('entriesBySection partitions the subnav', () => {
    const recombined = sectionOrder.flatMap((s) => entriesBySection(s))
    expect(recombined.length).toBe(subNav.length)
    // Every subnav entry appears in exactly one section bucket.
    for (const entry of subNav) {
      expect(recombined).toContain(entry)
    }
  })

  it('sections are non-empty', () => {
    for (const section of sectionOrder) {
      expect(entriesBySection(section).length).toBeGreaterThan(0)
    }
  })

  it('basics section contains general (the default landing page)', () => {
    const basics = entriesBySection('basics')
    expect(basics.some((e) => e.slug === DEFAULT_SLUG)).toBe(true)
  })
})
