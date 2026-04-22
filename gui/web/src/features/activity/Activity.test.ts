import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/svelte'

vi.mock('@/features/dashboard/resource.svelte', () => ({
  useTransportStats: () => ({ data: [] }),
  useSpeedStream: () => ({ data: { download: 0, upload: 0 } }),
  useSpeedHistory: () => ({ up: [], down: [] }),
}))

vi.mock('@/features/logs/store.svelte', () => ({
  logsStore: {
    subscribe: () => () => {},
    entries: [],
    filtered: [],
    text: '',
    activeConnectionCount: 0,
    clear: () => {},
    levels: { error: true, warn: true, info: true, debug: false },
    tags: {},
  },
}))

describe('Activity', () => {
  it('renders without crashing', async () => {
    const { default: Activity } = await import('./Activity.svelte')
    const { container } = render(Activity)
    expect(container.querySelector('h2, h3, h1')).toBeTruthy()
  })
})
