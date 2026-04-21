import { lazy } from '@/lib/router'
import type { AppRoute } from '@/app/routes'

export const route: AppRoute = {
  path: '/traffic',
  component: lazy(() => import('./Traffic.svelte')),
  nav: { label: 'nav.traffic', icon: 'traffic', order: 30 },
}
