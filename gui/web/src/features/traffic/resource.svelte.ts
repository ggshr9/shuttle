import { createResource, invalidate, type Resource } from '@/lib/resource.svelte'
import {
  getRouting,
  putRouting as apiPut,
  getRoutingTemplates,
  applyRoutingTemplate as apiApplyTpl,
  getGeositeCategories,
  getProcesses,
  importRouting as apiImport,
  testRouting as apiTest,
  exportRouting,
} from '@/lib/api/endpoints'
import type {
  RoutingRules, RoutingTemplate, Process, DryRunResult,
} from '@/lib/api/types'
import { toasts } from '@/lib/toaster.svelte'
import { t } from '@/lib/i18n/index'

const RULES_KEY = 'routing.rules'

export function useRules(): Resource<RoutingRules> {
  // No `initial` — wait for the first real fetch before rendering, so draft
  // initialization in RoutingPage sees actual rules, never the placeholder.
  // (A placeholder caused silent rule wipes when the user hit Save before
  // the first fetch landed.)
  // No `poll` — rules rarely change outside the page and a live poll would
  // compete with in-progress draft edits.
  return createResource(RULES_KEY, getRouting)
}

export async function saveRules(rules: RoutingRules): Promise<void> {
  try {
    await apiPut(rules)
    invalidate(RULES_KEY)
    toasts.success(t('routing.toast.saved'))
  } catch (e) {
    toasts.error((e as Error).message)
    throw e
  }
}

export function useTemplates(): Resource<RoutingTemplate[]> {
  return createResource('routing.templates', getRoutingTemplates, { initial: [] })
}

export function useCategories(): Resource<string[]> {
  return createResource('routing.geosite.categories', getGeositeCategories, {
    initial: [],
  })
}

export function useProcesses(): Resource<Process[]> {
  return createResource('routing.processes', getProcesses, { initial: [] })
}

export async function applyTemplate(id: string): Promise<void> {
  try {
    await apiApplyTpl(id)
    invalidate(RULES_KEY)
    toasts.success(t('routing.toast.templateApplied'))
  } catch (e) {
    toasts.error((e as Error).message)
    throw e
  }
}

export async function importRules(
  rules: RoutingRules,
  mode: 'merge' | 'replace',
): Promise<{ added: number; total: number } | null> {
  try {
    const r = await apiImport(rules, mode)
    invalidate(RULES_KEY)
    toasts.success(t('routing.toast.imported', { added: r.added, total: r.total }))
    return r
  } catch (e) {
    toasts.error((e as Error).message)
    return null
  }
}

export async function testUrl(url: string): Promise<DryRunResult | null> {
  try {
    return await apiTest(url)
  } catch (e) {
    toasts.error((e as Error).message)
    return null
  }
}

export const exportHref = exportRouting
