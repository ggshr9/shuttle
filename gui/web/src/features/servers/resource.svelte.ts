import { createResource, invalidate, type Resource } from '@/lib/resource.svelte'
import {
  getServers,
  addServer as apiAddServer,
  setActiveServer as apiSetActiveServer,
  deleteServer as apiDeleteServer,
  speedtest as apiSpeedtest,
  autoSelectServer as apiAutoSelect,
  importConfig as apiImport,
} from '@/lib/api/endpoints'
import type {
  Server, ServersResponse, SpeedtestResult, AutoSelectResult, ImportResult,
} from '@/lib/api/types'
import { toasts } from '@/lib/toaster.svelte'
import { t } from '@/lib/i18n/index'

const LIST_KEY = 'servers.list'

// ── Read ─────────────────────────────────────────────────────
export function useServers(): Resource<ServersResponse> {
  return createResource(
    LIST_KEY,
    getServers,
    { poll: 10_000, initial: { active: { addr: '' }, servers: [] } },
  )
}

// ── Mutations ────────────────────────────────────────────────
export async function addServer(srv: Server): Promise<void> {
  try {
    await apiAddServer(srv)
    invalidate(LIST_KEY)
    toasts.success(t('servers.toast.added', { name: srv.name || srv.addr }))
  } catch (e) {
    toasts.error((e as Error).message)
    throw e
  }
}

export async function setActive(srv: Server): Promise<void> {
  try {
    await apiSetActiveServer(srv)
    invalidate(LIST_KEY)
    invalidate('dashboard.status')
    toasts.success(t('servers.toast.switched', { name: srv.name || srv.addr }))
  } catch (e) {
    toasts.error((e as Error).message)
    throw e
  }
}

export async function removeServer(addr: string): Promise<void> {
  try {
    await apiDeleteServer(addr)
    invalidate(LIST_KEY)
  } catch (e) {
    toasts.error((e as Error).message)
    throw e
  }
}

export async function removeMany(addrs: string[]): Promise<void> {
  let ok = 0
  await Promise.allSettled(addrs.map(async (a) => {
    try {
      await apiDeleteServer(a)
      ok++
    } catch (e) {
      toasts.error(t('servers.toast.deleteFailed', { addr: a, msg: (e as Error).message }))
    }
  }))
  invalidate(LIST_KEY)
  if (ok > 0) {
    toasts.success(
      ok === 1
        ? t('servers.toast.deleted_one')
        : t('servers.toast.deleted_other', { n: ok })
    )
  }
}

export async function autoSelect(): Promise<AutoSelectResult | null> {
  try {
    const r = await apiAutoSelect()
    invalidate(LIST_KEY)
    invalidate('dashboard.status')
    toasts.success(t('servers.toast.autoSelected', {
      name: r.server.name || r.server.addr,
      latency: r.latency,
    }))
    return r
  } catch (e) {
    toasts.error((e as Error).message)
    return null
  }
}

export async function importServers(data: string): Promise<ImportResult | null> {
  try {
    const r = await apiImport(data)
    invalidate(LIST_KEY)
    if (r.error) {
      toasts.error(r.error)
    } else if (r.added > 0) {
      toasts.success(t('servers.toast.imported', { added: r.added, total: r.total }))
    } else {
      toasts.info(t('servers.toast.importedNone'))
    }
    return r
  } catch (e) {
    toasts.error((e as Error).message)
    return null
  }
}

// ── Speedtest results — transient, in-memory only ────────────
const results = $state<{ map: Record<string, SpeedtestResult> }>({ map: {} })

export function useSpeedtestResult(addr: string): SpeedtestResult | undefined {
  return results.map[addr]
}

export function getAllResults(): Record<string, SpeedtestResult> {
  return results.map
}

export async function runSpeedtest(addrs: string[]): Promise<void> {
  if (addrs.length === 0) return
  try {
    // The backend's POST /api/speedtest currently ignores the requested
    // addrs and returns results for every configured server. Filter client-
    // side so the toast count and stored results reflect the actual subset.
    // (A server-side fix belongs in a separate PR.)
    const wanted = new Set(addrs)
    const all = await apiSpeedtest(addrs)
    const rs = all.filter((r) => wanted.has(r.server_addr))
    for (const r of rs) results.map[r.server_addr] = r
    toasts.success(
      rs.length === 1
        ? t('servers.toast.tested_one')
        : t('servers.toast.tested_other', { n: rs.length })
    )
  } catch (e) {
    toasts.error((e as Error).message)
  }
}

export function __resetResults(): void {
  results.map = {}
}
