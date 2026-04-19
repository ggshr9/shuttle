import { describe, it, expect, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import Button from '@/ui/Button.svelte'

describe('Button', () => {
  it('fires onclick when enabled', async () => {
    const onclick = vi.fn()
    const { getByRole } = render(Button, { props: { onclick } })
    await fireEvent.click(getByRole('button'))
    expect(onclick).toHaveBeenCalled()
  })

  it('does not fire onclick when disabled', async () => {
    const onclick = vi.fn()
    const { getByRole } = render(Button, { props: { onclick, disabled: true } })
    await fireEvent.click(getByRole('button'))
    expect(onclick).not.toHaveBeenCalled()
  })

  it('adds loading class when loading', () => {
    const { getByRole } = render(Button, { props: { loading: true } })
    expect(getByRole('button').classList.contains('loading')).toBe(true)
  })
})
