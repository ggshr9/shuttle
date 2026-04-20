<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity'
  import { Empty, Card } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import SubscriptionRow from './SubscriptionRow.svelte'
  import SubscriptionRowExpanded from './SubscriptionRowExpanded.svelte'
  import type { Subscription } from '@/lib/api/types'

  interface Props {
    items: Subscription[]
    onDelete: (id: string) => void
  }
  let { items, onDelete }: Props = $props()

  // SvelteSet tracks add/delete mutations; plain Set in $state would not
  // trigger re-render on toggle.
  const expanded = new SvelteSet<string>()

  function toggle(id: string) {
    if (expanded.has(id)) expanded.delete(id)
    else expanded.add(id)
  }
</script>

{#if items.length === 0}
  <Card>
    <Empty
      icon="subscriptions"
      title={t('subscriptions.empty.title')}
      description={t('subscriptions.empty.desc')}
    />
  </Card>
{:else}
  <div class="table">
    <div class="header">
      <span></span>
      <span>{t('subscriptions.columns.name')}</span>
      <span>{t('subscriptions.columns.url')}</span>
      <span>{t('subscriptions.columns.servers')}</span>
      <span>{t('subscriptions.columns.updated')}</span>
      <span>{t('subscriptions.columns.format')}</span>
      <span></span>
    </div>
    {#each items as s (s.id)}
      <SubscriptionRow
        sub={s}
        expanded={expanded.has(s.id)}
        onExpandedToggle={() => toggle(s.id)}
        onDelete={() => onDelete(s.id)}
      />
      {#if expanded.has(s.id)}
        <SubscriptionRowExpanded sub={s} />
      {/if}
    {/each}
  </div>
{/if}

<style>
  .table {
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    background: var(--shuttle-bg-surface);
    overflow: hidden;
  }
  .header {
    display: grid;
    grid-template-columns: 16px 2fr 4fr 72px 120px 72px auto;
    align-items: center;
    gap: var(--shuttle-space-3);
    height: 36px;
    padding: 0 var(--shuttle-space-4);
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-muted);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    background: var(--shuttle-bg-subtle);
    border-bottom: 1px solid var(--shuttle-border);
  }
</style>
