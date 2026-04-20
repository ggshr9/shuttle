import { lazy } from '@/lib/router'
import type { AppRoute } from '@/app/routes'

export const route: AppRoute = {
  path: '/',
  component: lazy(() => import('./Dashboard.svelte')),
  nav: { label: 'nav.dashboard', icon: 'dashboard', order: 10 },
}

// Public hooks — other features can subscribe to the same status resource.
export { useStatus, useTransportStats } from './resource.svelte'
