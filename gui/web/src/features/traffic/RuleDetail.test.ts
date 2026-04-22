import { describe, it, expect, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import RuleDetail from './RuleDetail.svelte'

vi.mock('@/lib/api/endpoints', () => ({
  getGeositeCategories: vi.fn().mockResolvedValue(['google', 'netflix', 'cn']),
}))

describe('RuleDetail', () => {
  it('hydrates an existing domain rule', () => {
    const { container } = render(RuleDetail, {
      props: {
        initial: { domain: 'example.com', action: 'proxy' },
        onSave: vi.fn(),
        onCancel: vi.fn(),
      },
    })
    const input = container.querySelector('input[data-field="value"]') as HTMLInputElement
    expect(input?.value).toBe('example.com')
  })

  it('emits a normalized RoutingRule on save', async () => {
    const onSave = vi.fn()
    const { container, getByText } = render(RuleDetail, {
      props: {
        initial: { domain: 'example.com', action: 'proxy' },
        onSave,
        onCancel: vi.fn(),
      },
    })
    const input = container.querySelector('input[data-field="value"]') as HTMLInputElement
    await fireEvent.input(input, { target: { value: 'shuttle.dev' } })
    await fireEvent.click(getByText('Save'))
    expect(onSave).toHaveBeenCalledWith({ domain: 'shuttle.dev', action: 'proxy' })
  })

  it('emits cancel on cancel click', async () => {
    const onCancel = vi.fn()
    const { getByText } = render(RuleDetail, {
      props: {
        initial: { domain: '', action: 'proxy' },
        onSave: vi.fn(),
        onCancel,
      },
    })
    await fireEvent.click(getByText('Cancel'))
    expect(onCancel).toHaveBeenCalled()
  })
})
