import { lazy } from '@/lib/router'
import type { AppRoute } from '@/app/routes'

export const route: AppRoute = {
  path: '/routing',
  component: lazy(() => import('./RoutingPage.svelte')),
  nav: { label: 'nav.routing', icon: 'routing', order: 50 },
}

export { useRules } from './resource.svelte'
