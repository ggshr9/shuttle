import { lazy } from '@/lib/router'
import type { AppRoute } from '@/app/routes'

export const route: AppRoute = {
  path: '/',
  component: lazy(() => import('./Now.svelte')),
  nav: { label: 'nav.now', icon: 'power', order: 10 },
}
