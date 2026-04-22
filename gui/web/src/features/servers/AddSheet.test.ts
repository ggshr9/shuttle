import { describe, it, expect, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import AddSheet from './AddSheet.svelte'

vi.mock('@/lib/api/endpoints', () => ({
  addServer: vi.fn().mockResolvedValue(undefined),
  addSubscription: vi.fn().mockResolvedValue(undefined),
  importConfig: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/lib/resource.svelte', () => ({ invalidate: vi.fn() }))
vi.mock('@/lib/toaster.svelte', () => ({
  toasts: { success: vi.fn(), error: vi.fn(), info: vi.fn(), warning: vi.fn() },
}))
vi.mock('@/lib/platform', () => ({
  platform: {
    name: 'web',
    scanQRCode: vi.fn().mockResolvedValue('unsupported'),
  },
}))

describe('AddSheet', () => {
  it('renders all three method tabs', () => {
    const { getByText } = render(AddSheet, { props: { open: true } })
    expect(getByText('Manual')).toBeTruthy()
    expect(getByText('Paste')).toBeTruthy()
    expect(getByText('Subscribe')).toBeTruthy()
  })

  it('hides Scan QR on web runtime', () => {
    const { queryByText } = render(AddSheet, { props: { open: true } })
    expect(queryByText('Scan QR')).toBeNull()
  })

  it('switches to paste tab on click', async () => {
    const { getByText, queryByText } = render(AddSheet, { props: { open: true } })
    await fireEvent.click(getByText('Paste'))
    expect(queryByText(/Paste shuttle/)).toBeTruthy()
  })
})
