import { describe, it, expect } from 'vitest'
import { nav, primaryNav, navById } from './nav'

describe('nav', () => {
  it('has 6 items', () => { expect(nav.length).toBe(6) })
  it('5 are primary (for bottom tabs)', () => {
    expect(primaryNav().length).toBe(5)
  })
  it('settings is not primary', () => {
    expect(navById('settings')?.primary).toBe(false)
  })
  it('all items have unique IDs', () => {
    const ids = nav.map(i => i.id)
    expect(new Set(ids).size).toBe(ids.length)
  })
  it('sections cover overview / network / system', () => {
    const sections = new Set(nav.map(i => i.section))
    expect(sections.has('overview')).toBe(true)
    expect(sections.has('network')).toBe(true)
    expect(sections.has('system')).toBe(true)
  })
})
