<script lang="ts">
  import { Card, Badge, Empty } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import type { TransportStats } from '@/lib/api/types'

  interface Props { transports: TransportStats[] }
  let { transports }: Props = $props()

  const total = $derived(
    transports.reduce((sum, t) => sum + t.bytes_sent + t.bytes_recv, 0) || 1
  )

  function fmtBytes(t: TransportStats): string {
    const bytes = t.bytes_sent + t.bytes_recv
    if (bytes >= 1e9) return `${(bytes / 1e9).toFixed(1)} GB`
    if (bytes >= 1e6) return `${(bytes / 1e6).toFixed(1)} MB`
    if (bytes >= 1e3) return `${(bytes / 1e3).toFixed(0)} KB`
    return `${bytes} B`
  }

  function pct(t: TransportStats): number {
    return Math.max(0, Math.min(100, ((t.bytes_sent + t.bytes_recv) / total) * 100))
  }

  function stateOf(ts: TransportStats): { labelKey: string; variant: 'success' | 'neutral' } {
    if (ts.active_streams > 0) return { labelKey: 'dashboard.transportsPanel.state.primary', variant: 'success' }
    if (ts.total_streams > 0)  return { labelKey: 'dashboard.transportsPanel.state.standby', variant: 'neutral' }
    return { labelKey: 'dashboard.transportsPanel.state.idle', variant: 'neutral' }
  }

  const countLabel = $derived(
    transports.length === 1
      ? t('dashboard.transportsPanel.count_one',   { n: transports.length })
      : t('dashboard.transportsPanel.count_other', { n: transports.length })
  )
</script>

<Card>
  <header>
    <h3>{t('dashboard.transportsPanel.title')}</h3>
    <span class="count">{countLabel}</span>
  </header>

  {#if transports.length === 0}
    <Empty
      title={t('dashboard.transportsPanel.emptyTitle')}
      description={t('dashboard.transportsPanel.emptyDesc')}
    />
  {:else}
    <ul>
      {#each transports as ts}
        <li>
          <span class="name">{ts.transport}</span>
          <div class="bar"><div class="fill" style="width: {pct(ts)}%"></div></div>
          <span class="num">{fmtBytes(ts)}</span>
          <span class="num sm">{ts.active_streams} / {ts.total_streams}</span>
          <Badge variant={stateOf(ts).variant}>{t(stateOf(ts).labelKey)}</Badge>
        </li>
      {/each}
    </ul>
  {/if}
</Card>

<style>
  header {
    display: flex; align-items: center; gap: var(--shuttle-space-2);
    margin-bottom: var(--shuttle-space-3);
  }
  h3 { margin: 0; font-size: var(--shuttle-text-sm); font-weight: var(--shuttle-weight-semibold); color: var(--shuttle-fg-primary); }
  .count {
    margin-left: auto; font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-muted); font-family: var(--shuttle-font-mono);
  }

  ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 0; }
  li {
    display: grid;
    grid-template-columns: 80px 1fr 80px 72px auto;
    align-items: center; gap: var(--shuttle-space-3);
    padding: var(--shuttle-space-2) 0;
    border-top: 1px solid var(--shuttle-border);
    font-size: var(--shuttle-text-sm);
  }
  li:first-child { border-top: 0; }
  .name { font-family: var(--shuttle-font-mono); font-size: var(--shuttle-text-xs); color: var(--shuttle-fg-primary); }
  .bar { height: 4px; background: var(--shuttle-bg-subtle); border-radius: 2px; overflow: hidden; }
  .fill { height: 100%; background: var(--shuttle-accent); transition: width var(--shuttle-duration); }
  .num {
    color: var(--shuttle-fg-secondary); font-family: var(--shuttle-font-mono);
    font-size: var(--shuttle-text-xs); text-align: right;
  }
  .num.sm { color: var(--shuttle-fg-muted); }
</style>
