import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/svelte'
import ResponsiveGrid from './ResponsiveGrid.svelte'

describe('ResponsiveGrid', () => {
  it('applies cols per breakpoint as CSS custom properties', () => {
    const { container } = render(ResponsiveGrid, {
      props: { cols: { xs: 1, md: 2, lg: 3 } },
    })
    const el = container.querySelector('.grid') as HTMLElement
    expect(el.style.getPropertyValue('--cols-xs')).toBe('1')
    expect(el.style.getPropertyValue('--cols-md')).toBe('2')
    expect(el.style.getPropertyValue('--cols-lg')).toBe('3')
  })
})
