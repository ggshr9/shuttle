import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/svelte'
import { __resetRoute } from '@/lib/router/router.svelte'

vi.mock('@/lib/resources/status.svelte', () => ({
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
    levels: new Set(['error', 'warn', 'info']),
    toggleLevel: () => {},
    protocol: 'all',
    action: 'all',
    showConnections: false,
    autoScroll: true,
    tags: {},
    selected: null,        // LogDetail renders "Select a log entry" placeholder
    selectedId: null,
    select: () => {},
  },
}))

describe('Activity', () => {
  beforeEach(() => {
    location.hash = ''
    __resetRoute()
  })

  it('renders without crashing', async () => {
    const { default: Activity } = await import('./Activity.svelte')
    const { container } = render(Activity)
    expect(container.querySelector('h2, h3, h1')).toBeTruthy()
  })

  it('renders share button on logs tab', async () => {
    location.hash = '#/activity?tab=logs'
    __resetRoute()
    const { default: Activity } = await import('./Activity.svelte')
    const { container } = render(Activity)
    const btn = Array.from(container.querySelectorAll('button')).find(
      (b) => b.textContent?.includes('Share')
    )
    expect(btn).toBeTruthy()
  })
})
