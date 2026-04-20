import { createResource, invalidate, type Resource } from '@/lib/resource.svelte'
import {
  getSubscriptions,
  addSubscription as apiAdd,
  refreshSubscription as apiRefresh,
  deleteSubscription as apiDelete,
} from '@/lib/api/endpoints'
import type { Subscription } from '@/lib/api/types'
import { toasts } from '@/lib/toaster.svelte'
import { t } from '@/lib/i18n/index'

const LIST_KEY = 'subscriptions.list'

export function useSubscriptions(): Resource<Subscription[]> {
  return createResource(LIST_KEY, getSubscriptions, {
    poll: 30_000,
    initial: [],
  })
}

export async function addSubscription(name: string, url: string): Promise<void> {
  try {
    await apiAdd(name, url)
    invalidate(LIST_KEY)
    toasts.success(t('subscriptions.toast.added', { name: name || url }))
  } catch (e) {
    toasts.error((e as Error).message)
    throw e
  }
}

export async function refreshSubscription(id: string): Promise<void> {
  try {
    await apiRefresh(id)
    invalidate(LIST_KEY)
    toasts.success(t('subscriptions.toast.refreshed'))
  } catch (e) {
    toasts.error((e as Error).message)
  }
}

export async function deleteSubscription(id: string): Promise<void> {
  try {
    await apiDelete(id)
    invalidate(LIST_KEY)
  } catch (e) {
    toasts.error((e as Error).message)
    throw e
  }
}
