import { describe, it, expect, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import SourceFilter from './SourceFilter.svelte'

describe('SourceFilter', () => {
  it('renders All and given sources', () => {
    const { getByText } = render(SourceFilter, {
      props: {
        value: 'all',
        sources: [
          { id: 'manual', label: 'Manual' },
          { id: 'subscription:abc', label: 'sub:abc' },
        ],
        groups: [],
        onChange: vi.fn(),
      },
    })
    expect(getByText('All')).toBeTruthy()
    expect(getByText('Manual')).toBeTruthy()
    expect(getByText('sub:abc')).toBeTruthy()
  })

  it('fires onChange when chip clicked', async () => {
    const onChange = vi.fn()
    const { getByText } = render(SourceFilter, {
      props: {
        value: 'all',
        sources: [{ id: 'manual', label: 'Manual' }],
        groups: [],
        onChange,
      },
    })
    await fireEvent.click(getByText('Manual'))
    expect(onChange).toHaveBeenCalledWith('manual')
  })

  it('marks active chip with .active class', () => {
    const { getByText } = render(SourceFilter, {
      props: {
        value: 'manual',
        sources: [{ id: 'manual', label: 'Manual' }],
        groups: [],
        onChange: vi.fn(),
      },
    })
    const chip = getByText('Manual').closest('button')
    expect(chip?.classList.contains('active')).toBe(true)
  })
})
