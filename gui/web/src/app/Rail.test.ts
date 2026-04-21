import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/svelte'
import Rail from './Rail.svelte'
import { nav } from './nav'

describe('Rail', () => {
  it('renders every nav item (including non-primary)', () => {
    const { container } = render(Rail)
    const items = container.querySelectorAll('.rail-item')
    expect(items.length).toBe(nav.length)
  })
})
