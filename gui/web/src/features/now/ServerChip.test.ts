import { describe, it, expect, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import ServerChip from './ServerChip.svelte'

describe('ServerChip', () => {
  it('renders server name + transport', () => {
    const { getByText } = render(ServerChip, {
      props: { serverName: 'my-server', transport: 'H3', state: 'connected' },
    })
    expect(getByText(/my-server/)).toBeTruthy()
    expect(getByText(/H3/)).toBeTruthy()
  })
  it('invokes onClick when tapped', async () => {
    const onClick = vi.fn()
    const { container } = render(ServerChip, {
      props: { serverName: 'x', transport: '', state: 'disconnected', onClick },
    })
    await fireEvent.click(container.querySelector('button')!)
    expect(onClick).toHaveBeenCalled()
  })
})
