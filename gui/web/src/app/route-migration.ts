export interface Resolved {
  path: string
  query: Record<string, string>
}

export function resolveLegacyRoute(
  path: string,
  query: Record<string, string>,
): Resolved | null {
  // Dashboard: no deep subpaths; collapse any /dashboard/* to /.
  if (path === '/dashboard' || path.startsWith('/dashboard/')) {
    return { path: '/', query: { ...query } }
  }
  // Routing → Traffic: preserve tail so /routing/rules/:id → /traffic/rules/:id.
  if (path === '/routing') return { path: '/traffic', query: { ...query } }
  if (path.startsWith('/routing/')) {
    return { path: '/traffic' + path.slice('/routing'.length), query: { ...query } }
  }
  // Logs → Activity: Activity has no logs subroutes, so drop the tail and tag the tab.
  if (path === '/logs' || path.startsWith('/logs/')) {
    return { path: '/activity', query: { ...query, tab: 'logs' } }
  }
  if (path === '/subscriptions') {
    return { path: '/servers', query: { ...query, source: 'subscriptions' } }
  }
  if (path.startsWith('/subscriptions/')) {
    const id = path.slice('/subscriptions/'.length)
    return { path: '/servers', query: { ...query, source: `subscription:${id}` } }
  }
  // /groups → /servers (no view=groups tag — groups-by layout isn't scoped;
  // users land on the flat list where group chips in SourceFilter surface
  // their groups.)
  if (path === '/groups') {
    return { path: '/servers', query: { ...query } }
  }
  if (path.startsWith('/groups/')) {
    const id = path.slice('/groups/'.length)
    return { path: '/servers', query: { ...query, group: id } }
  }
  return null
}

const TOAST_KEY = 'shuttle-route-migration-seen'

export function hasSeenMigrationToast(): boolean {
  try { return localStorage.getItem(TOAST_KEY) === '1' } catch { return false }
}

export function markMigrationToastSeen(): void {
  try { localStorage.setItem(TOAST_KEY, '1') } catch {}
}
