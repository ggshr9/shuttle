import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/svelte'
import { __resetRoute } from '@/lib/router/router.svelte'

const { statusData, peersData } = vi.hoisted(() => ({
  statusData: {
    enabled: true as boolean,
    virtual_ip: '10.7.0.3',
    cidr: '10.7.0.0/24',
    peer_count: 2,
  },
  peersData: [] as unknown[],
}))

vi.mock('./resource.svelte', () => ({
  useStatus: () => ({
    data: statusData,
    loading: false,
    error: undefined,
    refetch: () => {},
  }),
  usePeers: () => ({
    data: peersData,
    loading: false,
    error: undefined,
    refetch: () => {},
  }),
}))

describe('MeshPage', () => {
  beforeEach(() => {
    statusData.enabled = true
    location.hash = '#/mesh'
    __resetRoute()
  })

  it('shows Peers tab by default', async () => {
    const { default: MeshPage } = await import('./MeshPage.svelte')
    const { container } = render(MeshPage)
    // Summary card (VirtualIP / CIDR / PeerCount) renders when enabled.
    expect(container.textContent).toContain('10.7.0.3')
  })

  it('shows spinner placeholder before TopologyChart loads', async () => {
    location.hash = '#/mesh?tab=topology'
    __resetRoute()
    const { default: MeshPage } = await import('./MeshPage.svelte')
    const { container } = render(MeshPage)
    // Dynamic import hasn't resolved in the test tick yet — .loading div is the placeholder.
    expect(container.querySelector('.loading')).toBeTruthy()
  })

  it('falls through to disabled card when mesh is off', async () => {
    statusData.enabled = false
    location.hash = '#/mesh'
    __resetRoute()
    const { default: MeshPage } = await import('./MeshPage.svelte')
    const { container } = render(MeshPage)
    // No tabs rendered when disabled; just the disabled card.
    expect(container.querySelector('[role="tablist"]')).toBeNull()
  })

  it('does NOT eagerly load TopologyChart on peers tab', async () => {
    // If lazy loading works, TopologyChart should not appear in the peers tab.
    location.hash = '#/mesh'
    __resetRoute()
    const { default: MeshPage } = await import('./MeshPage.svelte')
    const { container } = render(MeshPage)
    // TopologyChart renders an <svg> or canvas; .loading div only appears on the topology tab.
    expect(container.querySelector('.loading')).toBeNull()
  })
})
