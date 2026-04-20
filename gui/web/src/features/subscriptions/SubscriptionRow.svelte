<script lang="ts">
  import { Button, Icon, Badge } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { refreshSubscription } from './resource.svelte'
  import type { Subscription } from '@/lib/api/types'

  interface Props {
    sub: Subscription
    expanded: boolean
    onExpandedToggle: () => void
    onDelete: () => void
  }
  let { sub, expanded, onExpandedToggle, onDelete }: Props = $props()

  let refreshing = $state(false)
  async function refresh() {
    refreshing = true
    try { await refreshSubscription(sub.id) } finally { refreshing = false }
  }

  function inferFormat(url: string): string {
    if (/\.ya?ml(\?|$)/i.test(url)) return 'clash'
    if (/sip008|\.json(\?|$)/i.test(url)) return 'sip008'
    return 'auto'
  }

  function relative(ts: string | undefined): string {
    if (!ts) return t('subscriptions.never')
    const diff = Date.now() - new Date(ts).getTime()
    if (diff < 60_000)      return t('subscriptions.justNow')
    if (diff < 3_600_000)   return t('subscriptions.minutesAgo', { n: Math.floor(diff / 60_000) })
    if (diff < 86_400_000)  return t('subscriptions.hoursAgo',   { n: Math.floor(diff / 3_600_000) })
    return t('subscriptions.daysAgo', { n: Math.floor(diff / 86_400_000) })
  }

  const count = $derived(sub.servers?.length ?? 0)
  const status = $derived(sub.error ? 'bad' : sub.updated_at ? 'ok' : 'unknown')
</script>

<div class="row">
  <span class={`status ${status}`}></span>
  <span class="name">{sub.name || sub.url}</span>
  <span class="url">{sub.url}</span>
  <span class="count">{count}</span>
  <span class="ago">{relative(sub.updated_at)}</span>
  <span class="fmt"><Badge>{inferFormat(sub.url)}</Badge></span>
  <span class="actions">
    <Button size="sm" variant="ghost" onclick={onExpandedToggle}>
      <Icon name={expanded ? 'chevronDown' : 'chevronRight'} size={14} />
    </Button>
    <Button size="sm" variant="ghost" class="hover-only" loading={refreshing} onclick={refresh}>
      <Icon name="check" size={14} title={t('subscriptions.refresh')} />
    </Button>
    <Button size="sm" variant="ghost" onclick={onDelete}>
      <Icon name="trash" size={14} title={t('common.delete')} />
    </Button>
  </span>
</div>

<style>
  .row {
    display: grid;
    grid-template-columns: 16px 2fr 4fr 72px 120px 72px auto;
    align-items: center;
    gap: var(--shuttle-space-3);
    height: 48px;
    padding: 0 var(--shuttle-space-4);
    border-top: 1px solid var(--shuttle-border);
    font-size: var(--shuttle-text-sm);
  }
  .row:first-child { border-top: 0; }
  .status {
    width: 8px; height: 8px; border-radius: 50%;
    background: var(--shuttle-fg-muted);
  }
  .status.ok  { background: var(--shuttle-success); }
  .status.bad { background: var(--shuttle-danger); }
  .name { color: var(--shuttle-fg-primary); font-weight: var(--shuttle-weight-medium); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .url  { font-family: var(--shuttle-font-mono); color: var(--shuttle-fg-secondary); font-size: var(--shuttle-text-xs); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .count, .ago { color: var(--shuttle-fg-secondary); font-variant-numeric: tabular-nums; }
  .ago { font-size: var(--shuttle-text-xs); }
  .actions { display: flex; gap: 2px; justify-content: flex-end; }

  :global(.hover-only) { opacity: 0; transition: opacity var(--shuttle-duration); }
  .row:hover :global(.hover-only) { opacity: 1; }
</style>
