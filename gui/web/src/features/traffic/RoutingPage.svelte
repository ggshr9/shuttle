<script lang="ts">
  import { AsyncBoundary, Button, Section, Select } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { useRules, saveRules } from './resource.svelte'
  import RuleList from './RuleList.svelte'
  import RuleHitBar from './RuleHitBar.svelte'
  import TemplateDialog from './TemplateDialog.svelte'
  import ImportExportDialog from './ImportExportDialog.svelte'
  import TestPanel from './TestPanel.svelte'
  import type { RoutingRules, RoutingRule } from '@/lib/api/types'

  const res = useRules()

  let draft = $state<RoutingRules | null>(null)
  let saving = $state(false)
  let tplOpen = $state(false)
  let ioOpen = $state(false)

  // Initialize draft from remote on first real fetch. After template or
  // import mutations, callers set `draft = null` to force re-sync from the
  // newly-refreshed remote state (otherwise Save would overwrite the
  // freshly applied rules with the stale draft).
  $effect(() => {
    if (res.data && !draft) {
      draft = structuredClone(res.data)
    }
  })

  async function save() {
    if (!draft) return
    saving = true
    try {
      await saveRules(draft)
    } finally {
      saving = false
    }
  }

  function onRulesChange(rules: RoutingRule[]) {
    if (draft) draft = { ...draft, rules }
  }

  function onDefaultChange(v: string) {
    if (draft) draft = { ...draft, default: v }
  }

  function resetDraft() {
    draft = null    // Next $effect tick repopulates from res.data.
  }
</script>

<Section
  title={t('nav.traffic')}
  description={draft
    ? t('routing.count', { n: draft.rules.length })
    : res.data
      ? t('routing.count', { n: res.data.rules.length })
      : undefined}
>
  {#snippet actions()}
    <Button variant="ghost" onclick={() => (tplOpen = true)}>{t('routing.applyTemplate')}</Button>
    <Button variant="ghost" onclick={() => (ioOpen = true)}>{t('routing.importExport.open')}</Button>
    <Button variant="primary" loading={saving} onclick={save}>{t('common.save')}</Button>
  {/snippet}

  <AsyncBoundary resource={res}>
    {#snippet children(_remote)}
      {#if draft}
        <RuleHitBar rules={draft.rules} />

        <div class="default-row">
          <span class="label">{t('routing.default')}</span>
          <Select
            value={draft.default}
            options={[
              { value: 'proxy',  label: t('routing.action.proxy') },
              { value: 'direct', label: t('routing.action.direct') },
              { value: 'reject', label: t('routing.action.reject') },
            ]}
            onValueChange={onDefaultChange}
          />
        </div>

        <RuleList rules={draft.rules} onChange={onRulesChange} />

        <TestPanel />
      {/if}
    {/snippet}
  </AsyncBoundary>
</Section>

<TemplateDialog bind:open={tplOpen} onApplied={resetDraft} />
<ImportExportDialog bind:open={ioOpen} onImported={resetDraft} />

<style>
  .default-row {
    display: flex; align-items: center; gap: var(--shuttle-space-3);
    padding: var(--shuttle-space-3);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    background: var(--shuttle-bg-surface);
    margin-bottom: var(--shuttle-space-3);
  }
  .label {
    font-size: var(--shuttle-text-sm);
    color: var(--shuttle-fg-secondary);
  }
</style>
