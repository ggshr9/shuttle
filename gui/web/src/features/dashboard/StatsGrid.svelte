<script lang="ts">
  import { Card } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import type { Status, TransportStats } from '@/lib/api/types'

  interface Props {
    status: Status
    transports: TransportStats[]
  }
  let { status, transports }: Props = $props()

  const connected = $derived(status.connected === true)
  const active = $derived(
    transports.find((t) => t.active_streams > 0) ?? transports[0]
  )

  function formatBytes(n: number): string {
    if (n >= 1e9) return `${(n / 1e9).toFixed(1)} GB`
    if (n >= 1e6) return `${(n / 1e6).toFixed(1)} MB`
    if (n >= 1e3) return `${(n / 1e3).toFixed(0)} KB`
    return `${n} B`
  }

  const stats = $derived([
    {
      label: t('dashboard.stats.rtt'),
      value: connected ? `${(status as unknown as { rtt_ms?: number }).rtt_ms ?? '—'}` : '—',
      unit:  connected ? 'ms' : '',
      mono:  false,
    },
    {
      label: t('dashboard.stats.loss'),
      value: connected ? `${((status as unknown as { loss_rate?: number }).loss_rate ?? 0).toFixed(1)}` : '—',
      unit:  connected ? '%' : '',
      mono:  false,
    },
    {
      label: t('dashboard.stats.transfer'),
      value: connected ? formatBytes((status.bytes_sent ?? 0) + (status.bytes_recv ?? 0)) : '—',
      unit:  '',
      mono:  false,
    },
    {
      label: t('dashboard.transport'),
      value: connected ? (active?.transport ?? t('dashboard.stats.auto')) : '—',
      unit:  '',
      mono:  true,
    },
  ])
</script>

<div class="grid">
  {#each stats as s}
    <Card>
      <div class="lbl">{s.label}</div>
      <div class="val" class:mono={s.mono}>
        {s.value}{#if s.unit}<span class="unit"> {s.unit}</span>{/if}
      </div>
    </Card>
  {/each}
</div>

<style>
  .grid {
    display: grid; grid-template-columns: repeat(4, 1fr);
    gap: var(--shuttle-space-3);
  }
  .lbl {
    font-size: var(--shuttle-text-xs); color: var(--shuttle-fg-muted);
    text-transform: uppercase; letter-spacing: 0.08em;
  }
  .val {
    font-size: var(--shuttle-text-xl); font-weight: var(--shuttle-weight-semibold);
    letter-spacing: var(--shuttle-tracking-tight);
    margin-top: var(--shuttle-space-1);
    font-variant-numeric: tabular-nums;
    color: var(--shuttle-fg-primary);
  }
  .val.mono { font-family: var(--shuttle-font-mono); font-size: var(--shuttle-text-base); padding-top: 4px; }
  .unit { font-size: var(--shuttle-text-sm); color: var(--shuttle-fg-muted); font-weight: var(--shuttle-weight-regular); }

  @media (max-width: 860px) {
    .grid { grid-template-columns: repeat(2, 1fr); }
  }
</style>
