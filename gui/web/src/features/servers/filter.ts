// Pure source-filter logic for the Servers page. Extracted from
// ServersPage.svelte so the branches can be unit-tested without a
// reactive Svelte render.

import type { Server, Subscription, GroupInfo } from '@/lib/api/types'

/**
 * Filter value shapes (all strings, URL-friendly):
 *   'all'                 — every server
 *   'manual'              — servers NOT owned by any subscription
 *   'subscriptions'       — servers owned by at least one subscription
 *   'subscription:<id>'   — servers inside a specific subscription
 *   'group:<tag>'         — servers listed as members of a specific group
 */
export type SourceFilter = string

/** Collect every server addr that is owned by any subscription. */
export function subServerAddrs(subs: readonly Subscription[]): Set<string> {
  return new Set(
    subs.flatMap((s) => s.servers ?? []).map((s) => s.addr)
  )
}

/**
 * Decide whether `srv` should render under `filter`. `subs` + `groups` are
 * passed in (not looked up) so this stays testable without stubs — a
 * misconfigured filter string returns `true` (include everything) rather
 * than silently hiding the list.
 */
export function matchesFilter(
  srv: Server,
  filter: SourceFilter,
  subs: readonly Subscription[],
  groups: readonly GroupInfo[],
): boolean {
  if (filter === 'all') return true

  if (filter === 'manual') return !subServerAddrs(subs).has(srv.addr)
  if (filter === 'subscriptions') return subServerAddrs(subs).has(srv.addr)

  if (filter.startsWith('subscription:')) {
    const id = filter.slice('subscription:'.length)
    const sub = subs.find((s) => s.id === id)
    return !!sub?.servers?.some((s) => s.addr === srv.addr)
  }

  if (filter.startsWith('group:')) {
    const tag = filter.slice('group:'.length)
    const group = groups.find((g) => g.tag === tag)
    return !!group?.members?.includes(srv.addr)
  }

  return true
}
