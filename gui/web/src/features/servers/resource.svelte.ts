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
    toasts.success(`Added ${srv.name || srv.addr}`)
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
    toasts.success(`Switched to ${srv.name || srv.addr}`)
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
  for (const a of addrs) {
    try {
      await apiDeleteServer(a)
    } catch (e) {
      toasts.error(`Failed to delete ${a}: ${(e as Error).message}`)
    }
  }
  invalidate(LIST_KEY)
  toasts.success(`Deleted ${addrs.length} ${addrs.length === 1 ? 'server' : 'servers'}`)
}

export async function autoSelect(): Promise<AutoSelectResult | null> {
  try {
    const r = await apiAutoSelect()
    invalidate(LIST_KEY)
    invalidate('dashboard.status')
    toasts.success(`Auto-selected ${r.server.name || r.server.addr} (${r.latency} ms)`)
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
      toasts.success(`Imported ${r.added} of ${r.total} servers`)
    } else {
      toasts.info('No new servers imported')
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
    const rs = await apiSpeedtest(addrs)
    for (const r of rs) results.map[r.server_addr] = r
    toasts.success(`Tested ${rs.length} ${rs.length === 1 ? 'server' : 'servers'}`)
  } catch (e) {
    toasts.error((e as Error).message)
  }
}

export function __resetResults(): void {
  results.map = {}
}
