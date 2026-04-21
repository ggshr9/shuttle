import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import TopBar from './TopBar.svelte'

describe('TopBar', () => {
  it('renders title passed via prop', () => {
    render(TopBar, { props: { title: 'Servers' } })
    expect(screen.getByText('Servers')).toBeTruthy()
  })
  it('renders settings gear with aria-label', () => {
    const { container } = render(TopBar, { props: { title: 'X' } })
    const gear = container.querySelector('[aria-label="Settings"]')
    expect(gear).toBeTruthy()
  })
})
