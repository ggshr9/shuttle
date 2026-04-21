import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/svelte'
import Now from './Now.svelte'

vi.mock('@/lib/platform', () => ({
  platform: {
    name: 'web',
    engineStart: vi.fn(),
    engineStop: vi.fn(),
    engineStatus: vi.fn().mockResolvedValue({ connected: false, server: null }),
    requestVpnPermission: vi.fn().mockResolvedValue('unsupported'),
    onStatusChange: () => () => {},
  },
}))

vi.mock('@/features/dashboard/resource.svelte', () => ({
  useStatus: () => ({ data: { connected: false, uptime: 0, server: null }, loading: false, error: undefined, refetch: () => {} }),
  useSpeedStream: () => ({ data: { download: 0, upload: 0 } }),
  useTransportStats: () => ({ data: [] }),
}))

describe('Now', () => {
  it('renders a power button', () => {
    const { container } = render(Now)
    expect(container.querySelector('[role="switch"]')).toBeTruthy()
  })
})
