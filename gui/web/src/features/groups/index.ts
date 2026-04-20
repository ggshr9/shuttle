import { lazy } from '@/lib/router'
import type { AppRoute } from '@/app/routes'

const loadPage = lazy(() => import('./GroupsPage.svelte'))

// Two routes share GroupsPage — it branches internally via
// matches('/groups/:tag'). Both must exist because RouterOutlet's matchPath
// is strict-length; a standalone /groups entry would leave /groups/<tag>
// unrouted, leaving the main area blank (Codex P2 finding).
export const route: AppRoute = {
  path: '/groups',
  component: loadPage,
  nav: { label: 'nav.groups', icon: 'groups', order: 40 },
}

export const detailRoute: AppRoute = {
  path: '/groups/:tag',
  component: loadPage,
  nav: { label: 'nav.groups', icon: 'groups', order: 40, hidden: true },
}

export { useGroups } from './resource.svelte'
