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

  it('only the active chip is in the tab order (roving tabindex)', () => {
    const { getByText } = render(SourceFilter, {
      props: {
        value: 'manual',
        sources: [{ id: 'manual', label: 'Manual' }, { id: 'subs', label: 'Subs' }],
        groups: [],
        onChange: vi.fn(),
      },
    })
    const all = getByText('All').closest('button')!
    const manual = getByText('Manual').closest('button')!
    const subs = getByText('Subs').closest('button')!
    expect(all.getAttribute('tabindex')).toBe('-1')
    expect(manual.getAttribute('tabindex')).toBe('0')
    expect(subs.getAttribute('tabindex')).toBe('-1')
  })

  it('ArrowRight moves selection to the next chip', async () => {
    const onChange = vi.fn()
    const { getByText } = render(SourceFilter, {
      props: {
        value: 'all',
        sources: [{ id: 'manual', label: 'Manual' }],
        groups: [],
        onChange,
      },
    })
    const allBtn = getByText('All').closest('button')!
    await fireEvent.keyDown(allBtn, { key: 'ArrowRight' })
    expect(onChange).toHaveBeenCalledWith('manual')
  })

  it('ArrowLeft wraps around to the last chip', async () => {
    const onChange = vi.fn()
    const { getByText } = render(SourceFilter, {
      props: {
        value: 'all',
        sources: [{ id: 'manual', label: 'Manual' }, { id: 'subs', label: 'Subs' }],
        groups: [],
        onChange,
      },
    })
    const allBtn = getByText('All').closest('button')!
    await fireEvent.keyDown(allBtn, { key: 'ArrowLeft' })
    expect(onChange).toHaveBeenCalledWith('subs')
  })

  it('Home / End jump to first / last chip', async () => {
    const onChange = vi.fn()
    const { getByText } = render(SourceFilter, {
      props: {
        value: 'manual',
        sources: [{ id: 'manual', label: 'Manual' }, { id: 'subs', label: 'Subs' }],
        groups: [],
        onChange,
      },
    })
    const manualBtn = getByText('Manual').closest('button')!
    await fireEvent.keyDown(manualBtn, { key: 'End' })
    expect(onChange).toHaveBeenLastCalledWith('subs')
    await fireEvent.keyDown(manualBtn, { key: 'Home' })
    expect(onChange).toHaveBeenLastCalledWith('all')
  })
})
