import { describe, it, expect, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import SubscriptionBanner from './SubscriptionBanner.svelte'

vi.mock('@/lib/api/endpoints', () => ({
  refreshSubscription: vi.fn().mockResolvedValue(undefined),
  deleteSubscription: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/lib/resource.svelte', () => ({
  invalidate: vi.fn(),
}))

vi.mock('@/lib/toaster.svelte', () => ({
  toasts: { success: vi.fn(), error: vi.fn() },
}))

describe('SubscriptionBanner', () => {
  it('renders subscription meta', () => {
    const { getByText } = render(SubscriptionBanner, {
      props: {
        sub: {
          id: 'abc',
          name: 'My Sub',
          url: 'https://example.com/feed',
          servers: [{ addr: '1.1.1.1:443' }, { addr: '2.2.2.2:443' }],
          updated_at: '2026-04-20T10:00:00Z',
        },
      },
    })
    expect(getByText('My Sub')).toBeTruthy()
    expect(getByText('https://example.com/feed')).toBeTruthy()
    expect(getByText(/2 servers/)).toBeTruthy()
  })

  it('refresh button calls refreshSubscription endpoint', async () => {
    const endpoints = await import('@/lib/api/endpoints')
    const { getByText } = render(SubscriptionBanner, {
      props: {
        sub: { id: 'xyz', name: 'S', url: 'u', servers: [] },
      },
    })
    await fireEvent.click(getByText('Refresh'))
    expect(endpoints.refreshSubscription).toHaveBeenCalledWith('xyz')
  })
})
