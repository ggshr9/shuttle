import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/svelte'
import Stack from './Stack.svelte'

describe('Stack', () => {
  it('defaults to column layout', () => {
    const { container } = render(Stack, { props: { gap: '3' } })
    const el = container.querySelector('.stack') as HTMLElement
    expect(el.dataset.direction).toBe('column')
  })

  it('applies breakAt for horizontal-above-breakpoint', () => {
    const { container } = render(Stack, { props: { breakAt: 'md' } })
    const el = container.querySelector('.stack') as HTMLElement
    expect(el.dataset.breakAt).toBe('md')
  })
})
