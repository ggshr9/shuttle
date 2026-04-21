<script lang="ts">
  import { Card, Input, Button, Badge } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { testUrl } from './resource.svelte'
  import type { DryRunResult } from '@/lib/api/types'

  let url = $state('')
  let busy = $state(false)
  let result = $state<DryRunResult | null>(null)

  async function run() {
    if (!url.trim()) return
    busy = true
    try {
      result = await testUrl(url.trim())
    } finally {
      busy = false
    }
  }

  const variant = $derived<'success' | 'info' | 'danger' | 'neutral'>(
    !result ? 'neutral'
      : result.action === 'proxy'  ? 'info'
      : result.action === 'direct' ? 'success'
      : 'danger'
  )
</script>

<Card>
  <h3>{t('routing.test.title')}</h3>
  <div class="row">
    <Input placeholder={t('routing.test.placeholder')} bind:value={url} />
    <Button variant="primary" loading={busy} onclick={run}>{t('routing.test.test')}</Button>
  </div>

  {#if result}
    <div class="result">
      <Badge variant={variant === 'neutral' ? undefined : variant}>{result.action}</Badge>
      <span class="domain">{result.domain}</span>
      <span class="matched">
        {t('routing.test.matchedBy', { rule: result.matched_by, detail: result.rule ?? '' })}
      </span>
    </div>
  {/if}
</Card>

<style>
  h3 {
    margin: 0 0 var(--shuttle-space-3);
    font-size: var(--shuttle-text-sm);
    font-weight: var(--shuttle-weight-semibold);
    color: var(--shuttle-fg-primary);
  }
  .row { display: grid; grid-template-columns: 1fr auto; gap: var(--shuttle-space-2); align-items: end; }
  .result {
    display: flex; align-items: center; gap: var(--shuttle-space-2);
    margin-top: var(--shuttle-space-3);
    font-size: var(--shuttle-text-sm);
  }
  .domain { font-family: var(--shuttle-font-mono); color: var(--shuttle-fg-primary); }
  .matched { color: var(--shuttle-fg-muted); font-size: var(--shuttle-text-xs); margin-left: auto; }
</style>
