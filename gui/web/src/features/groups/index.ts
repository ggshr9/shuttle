import { lazy } from '@/lib/router'
import type { AppRoute } from '@/app/routes'

export const route: AppRoute = {
  path: '/groups',
  component: lazy(() => import('./GroupsPage.svelte')),
  nav: { label: 'nav.groups', icon: 'groups', order: 40 },
}

export { useGroups } from './resource.svelte'
