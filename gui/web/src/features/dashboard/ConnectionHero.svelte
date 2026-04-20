<script lang="ts">
  import { Card, Button } from '@/ui'
  import { navigate } from '@/lib/router'
  import { connect, disconnect } from '@/lib/api/endpoints'
  import { invalidate } from '@/lib/resource.svelte'
  import { toasts } from '@/lib/toaster.svelte'
  import type { Status } from '@/lib/api/types'

  interface Props { status: Status }
  let { status }: Props = $props()

  const connected = $derived(status.connected === true)
  const serverLabel = $derived(status.server?.name || status.server?.addr || '—')
  const transportLabel = $derived(
    (status as unknown as { transport?: string }).transport ?? 'auto'
  )

  function formatUptime(s: number): string {
    if (s < 60) return `${s}s`
    const m = Math.floor(s / 60)
    if (m < 60) return `${m}m ${s % 60}s`
    const h = Math.floor(m / 60)
    return `${h}h ${m % 60}m`
  }

  function formatSpeed(bps: number): { value: string; unit: string } {
    if (bps >= 1e6) return { value: (bps / 1e6).toFixed(1), unit: 'MB/s' }
    if (bps >= 1e3) return { value: (bps / 1e3).toFixed(1), unit: 'KB/s' }
    return { value: String(bps), unit: 'B/s' }
  }

  const uptime = $derived(formatUptime(status.uptime ?? 0))
  const down = $derived(formatSpeed(status.bytes_recv ?? 0))
  const up   = $derived(formatSpeed(status.bytes_sent ?? 0))

  let busy = $state(false)
  async function toggle() {
    busy = true
    try {
      if (connected) await disconnect()
      else await connect()
      invalidate('dashboard.status')
    } catch (e) {
      toasts.error((e as Error).message)
    } finally {
      busy = false
    }
  }
</script>

<Card>
  <div class="hero">
    <div class="head">
      <span class="dot" class:on={connected}></span>
      <span class="state">
        {#if connected}
          Connected · <b>{serverLabel}</b> via {transportLabel}
        {:else}
          Disconnected
        {/if}
      </span>
      <span class="spacer"></span>
      {#if connected}<span class="uptime">{uptime}</span>{/if}
    </div>

    <div class="row">
      <div class="speed">
        <div class="big">
          {down.value}<span class="unit"> {down.unit}</span>
        </div>
        <div class="label">Download</div>
      </div>
      <div class="speed small">
        <div class="mid">{up.value}</div>
        <div class="label">Upload {up.unit}</div>
      </div>
      <div class="spacer"></div>
      <div class="actions">
        <Button variant="primary" loading={busy} onclick={toggle}>
          {connected ? 'Disconnect' : 'Connect'}
        </Button>
        <Button variant="ghost" onclick={() => navigate('/servers')}>
          Switch server
        </Button>
      </div>
    </div>
  </div>
</Card>

<style>
  .hero { display: flex; flex-direction: column; gap: var(--shuttle-space-4); }
  .head { display: flex; align-items: center; gap: var(--shuttle-space-2); }
  .dot  { width: 8px; height: 8px; border-radius: 50%; background: var(--shuttle-fg-muted); }
  .dot.on { background: var(--shuttle-success); }
  .state { font-size: var(--shuttle-text-sm); color: var(--shuttle-fg-secondary); }
  .state b { color: var(--shuttle-fg-primary); font-weight: var(--shuttle-weight-semibold); }
  .spacer { flex: 1; }
  .uptime { font-family: var(--shuttle-font-mono); font-size: var(--shuttle-text-sm); color: var(--shuttle-fg-secondary); }

  .row { display: flex; align-items: baseline; gap: var(--shuttle-space-5); }
  .speed .big {
    font-size: var(--shuttle-text-2xl); font-weight: var(--shuttle-weight-semibold);
    letter-spacing: var(--shuttle-tracking-tight); line-height: 1;
    font-variant-numeric: tabular-nums; color: var(--shuttle-fg-primary);
  }
  .speed .mid {
    font-size: var(--shuttle-text-xl); color: var(--shuttle-fg-secondary);
    font-variant-numeric: tabular-nums;
  }
  .speed .label {
    font-size: var(--shuttle-text-xs); color: var(--shuttle-fg-muted);
    text-transform: uppercase; letter-spacing: 0.08em; margin-top: var(--shuttle-space-1);
  }
  .unit { font-size: var(--shuttle-text-lg); color: var(--shuttle-fg-muted); }
  .actions { display: flex; gap: var(--shuttle-space-2); }
</style>
