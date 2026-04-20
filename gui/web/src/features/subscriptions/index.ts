import { lazy } from '@/lib/router'
import type { AppRoute } from '@/app/routes'

export const route: AppRoute = {
  path: '/subscriptions',
  component: lazy(() => import('./SubscriptionsPage.svelte')),
  nav: { label: 'nav.subscriptions', icon: 'subscriptions', order: 30 },
}

export { useSubscriptions } from './resource.svelte'
