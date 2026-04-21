import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import BottomTabs from './BottomTabs.svelte'
import { primaryNav } from './nav'

describe('BottomTabs', () => {
  it('renders one tab per primary nav item', () => {
    const { container } = render(BottomTabs)
    const tabs = container.querySelectorAll('[role="tab"]')
    expect(tabs.length).toBe(primaryNav().length)
  })
  it('has aria-label on the nav container', () => {
    const { container } = render(BottomTabs)
    const nav = container.querySelector('[aria-label="Primary navigation"]')
    expect(nav).toBeTruthy()
  })
})
