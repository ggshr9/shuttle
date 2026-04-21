import { lazy } from '@/lib/router'
import type { AppRoute } from '@/app/routes'
import { subNav } from './nav'

const component = lazy(() => import('./SettingsPage.svelte'))

export const route: AppRoute = {
  path: '/settings',
  component,
  nav: { label: 'nav.settings', icon: 'settings', order: 90 },
  children: subNav.map((n) => ({ path: n.slug, component })),
}

export { settings } from './config.svelte'
