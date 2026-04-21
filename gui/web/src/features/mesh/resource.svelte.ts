import { createResource, invalidate, type Resource } from '@/lib/resource.svelte'
import {
  meshStatus as apiStatus,
  meshPeers as apiPeers,
  meshConnectPeer as apiConnect,
} from '@/lib/api/endpoints'
import type { MeshStatus, MeshPeer } from '@/lib/api/types'
import { toasts } from '@/lib/toaster.svelte'
import { t } from '@/lib/i18n/index'

const STATUS_KEY = 'mesh.status'
const PEERS_KEY  = 'mesh.peers'

export function useStatus(): Resource<MeshStatus> {
  return createResource(STATUS_KEY, apiStatus, {
    poll: 10_000,
    initial: { enabled: false },
  })
}

export function usePeers(): Resource<MeshPeer[]> {
  return createResource(PEERS_KEY, apiPeers, {
    poll: 10_000,
    initial: [],
    enabled: () => useStatus().data?.enabled === true,
  })
}

export async function connectPeer(vip: string): Promise<void> {
  try {
    await apiConnect(vip)
    invalidate(PEERS_KEY)
    toasts.success(t('mesh.toast.connecting', { vip }))
  } catch (e) {
    toasts.error((e as Error).message)
    throw e
  }
}
