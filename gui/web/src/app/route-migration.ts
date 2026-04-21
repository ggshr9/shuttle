export interface Resolved {
  path: string
  query: Record<string, string>
}

export function resolveLegacyRoute(
  path: string,
  query: Record<string, string>,
): Resolved | null {
  if (path === '/dashboard')   return { path: '/',         query: { ...query } }
  if (path === '/routing')     return { path: '/traffic',  query: { ...query } }
  if (path === '/logs')        return { path: '/activity', query: { tab: 'logs', ...query } }
  if (path === '/subscriptions') {
    return { path: '/servers', query: { source: 'subscriptions', ...query } }
  }
  if (path.startsWith('/subscriptions/')) {
    const id = path.slice('/subscriptions/'.length)
    return { path: '/servers', query: { source: `subscription:${id}`, ...query } }
  }
  if (path === '/groups') {
    return { path: '/servers', query: { view: 'groups', ...query } }
  }
  if (path.startsWith('/groups/')) {
    const id = path.slice('/groups/'.length)
    return { path: '/servers', query: { group: id, ...query } }
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
