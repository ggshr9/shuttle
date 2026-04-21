import { lazy } from '@/lib/router'
import type { AppRoute } from '@/app/routes'

export const route: AppRoute = {
  path: '/logs',
  component: lazy(() => import('./LogsPage.svelte')),
  nav: { label: 'nav.logs', icon: 'logs', order: 80 },
}

export { logsStore } from './store.svelte'
