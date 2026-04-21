import { describe, it, expect, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import PowerButton from './PowerButton.svelte'

describe('PowerButton', () => {
  it('renders disconnected state by default', () => {
    const { container } = render(PowerButton, { props: { state: 'disconnected' } })
    const btn = container.querySelector('[role="switch"]') as HTMLElement
    expect(btn.dataset.state).toBe('disconnected')
    expect(btn.getAttribute('aria-checked')).toBe('false')
  })

  it('renders connected state with aria-checked=true', () => {
    const { container } = render(PowerButton, { props: { state: 'connected' } })
    const btn = container.querySelector('[role="switch"]') as HTMLElement
    expect(btn.getAttribute('aria-checked')).toBe('true')
  })

  it('disables while connecting', () => {
    const { container } = render(PowerButton, { props: { state: 'connecting' } })
    const btn = container.querySelector('[role="switch"]') as HTMLButtonElement
    expect(btn.disabled).toBe(true)
  })

  it('invokes onToggle on click', async () => {
    const onToggle = vi.fn()
    const { container } = render(PowerButton, {
      props: { state: 'disconnected', onToggle },
    })
    const btn = container.querySelector('[role="switch"]') as HTMLButtonElement
    await fireEvent.click(btn)
    expect(onToggle).toHaveBeenCalled()
  })
})
