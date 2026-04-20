import { createResource, invalidate, type Resource } from '@/lib/resource.svelte'
import {
  getGroups,
  getGroup as apiGetGroup,
  testGroup as apiTestGroup,
  selectGroupMember as apiSelectMember,
} from '@/lib/api/endpoints'
import type { GroupInfo, GroupTestResult } from '@/lib/api/types'
import { toasts } from '@/lib/toaster.svelte'
import { t } from '@/lib/i18n/index'

const LIST_KEY = 'groups.list'

export function useGroups(): Resource<GroupInfo[]> {
  return createResource(LIST_KEY, getGroups, {
    poll: 15_000,
    initial: [],
  })
}

// Single-group resource keyed by tag. Each tag gets its own cached entry.
// P11 may add eviction when many tags have been visited.
export function useGroup(tag: string): Resource<GroupInfo> {
  return createResource(
    `groups.item.${tag}`,
    () => apiGetGroup(tag),
    { poll: 10_000 },
  )
}

// Test results are transient per-group; stored in a small module map.
const testResults = $state<{ map: Record<string, GroupTestResult[]> }>({ map: {} })

export function useGroupTestResults(tag: string): GroupTestResult[] {
  return testResults.map[tag] ?? []
}

export async function testGroup(tag: string): Promise<void> {
  try {
    const rs = await apiTestGroup(tag)
    testResults.map[tag] = rs
    toasts.success(t('groups.toast.tested', { n: rs.length }))
  } catch (e) {
    toasts.error((e as Error).message)
  }
}

export async function selectMember(groupTag: string, member: string): Promise<void> {
  try {
    await apiSelectMember(groupTag, member)
    invalidate(LIST_KEY)
    invalidate(`groups.item.${groupTag}`)
    toasts.success(t('groups.toast.selected', { name: member }))
  } catch (e) {
    toasts.error((e as Error).message)
    throw e
  }
}

export function __resetGroupResults(): void {
  testResults.map = {}
}
