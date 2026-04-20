import { lazy } from '@/lib/router'
import type { AppRoute } from '@/app/routes'

export const route: AppRoute = {
  path: '/servers',
  component: lazy(() => import('./ServersPage.svelte')),
  nav: { label: 'nav.servers', icon: 'servers', order: 20 },
}

export { useServers, setActive } from './resource.svelte'
