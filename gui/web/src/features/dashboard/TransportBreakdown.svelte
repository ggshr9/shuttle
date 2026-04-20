<script lang="ts">
  import { Card, Badge, Empty } from '@/ui'
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

  function state(t: TransportStats): { label: string; variant: 'success' | 'neutral' } {
    if (t.active_streams > 0) return { label: 'PRIMARY', variant: 'success' }
    if (t.total_streams > 0)  return { label: 'STANDBY', variant: 'neutral' }
    return { label: 'IDLE', variant: 'neutral' }
  }
</script>

<Card>
  <header>
    <h3>Active transports</h3>
    <span class="count">
      {transports.length} {transports.length === 1 ? 'transport' : 'transports'}
    </span>
  </header>

  {#if transports.length === 0}
    <Empty title="No transport data" description="Connect to see per-transport breakdown." />
  {:else}
    <ul>
      {#each transports as t}
        <li>
          <span class="name">{t.transport}</span>
          <div class="bar"><div class="fill" style="width: {pct(t)}%"></div></div>
          <span class="num">{fmtBytes(t)}</span>
          <span class="num sm">{t.active_streams} / {t.total_streams}</span>
          <Badge variant={state(t).variant}>{state(t).label}</Badge>
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
