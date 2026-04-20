import { describe, it, expect, beforeEach } from 'vitest'
import { render } from '@testing-library/svelte'
import { __resetRoute } from '@/lib/router/router.svelte'
import Sidebar from '@/app/Sidebar.svelte'
import type { AppRoute } from '@/app/routes'

// Stub components: `lazy()` loader returning null keeps routes lightweight
// and avoids pulling legacy pages into the unit test.
const stub = () => Promise.resolve(null as unknown as any)

const mockRoutes: AppRoute[] = [
  { path: '/',        component: stub, nav: { label: 'nav.dashboard', icon: 'dashboard', order: 10 } },
  { path: '/servers', component: stub, nav: { label: 'nav.servers',   icon: 'servers',   order: 20 } },
  { path: '/logs',    component: stub, nav: { label: 'nav.logs',      icon: 'logs',      order: 80 } },
]

describe('Sidebar', () => {
  beforeEach(() => {
    location.hash = ''
    __resetRoute()
  })

  it('renders one anchor per route with nav metadata', () => {
    const { container } = render(Sidebar, { props: { routes: mockRoutes } })
    const links = container.querySelectorAll('a.item')
    expect(links.length).toBe(3)
  })

  it('groups entries into three sections (overview / network / system)', () => {
    const { container } = render(Sidebar, { props: { routes: mockRoutes } })
    const headings = container.querySelectorAll('.heading')
    expect(headings.length).toBe(3)
  })

  it('hides labels when collapsed', () => {
    const { container } = render(Sidebar, { props: { routes: mockRoutes, collapsed: true } })
    const headings = container.querySelectorAll('.heading')
    expect(headings.length).toBe(0)
  })
})
