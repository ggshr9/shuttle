import { lazy } from '@/lib/router'
import type { AppRoute } from '@/app/routes'

export const route: AppRoute = {
  path: '/mesh',
  component: lazy(() => import('./MeshPage.svelte')),
  nav: { label: 'nav.mesh', icon: 'mesh', order: 60 },
}

export { useStatus, usePeers } from './resource.svelte'
