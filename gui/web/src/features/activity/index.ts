import { lazy } from '@/lib/router'
import type { AppRoute } from '@/app/routes'

export const route: AppRoute = {
  path: '/activity',
  component: lazy(() => import('./Activity.svelte')),
  nav: { label: 'nav.activity', icon: 'activity', order: 50 },
}
