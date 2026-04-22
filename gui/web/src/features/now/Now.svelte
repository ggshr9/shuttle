<script lang="ts">
  import PowerButton from './PowerButton.svelte'
  import ServerChip from './ServerChip.svelte'
  import { platform } from '@/lib/platform'
  import { toasts } from '@/lib/toaster.svelte'
  import { navigate } from '@/lib/router'
  import { useStatus, useSpeedStream } from '@/lib/resources/status.svelte'
  import { t } from '@/lib/i18n/index'
  import { AsyncBoundary } from '@/ui'
  import { invalidate } from '@/lib/resource.svelte'
  import { errorMessage } from '@/lib/format'
  import type { Status } from '@/lib/api/types'

  type PowerState = 'disconnected' | 'connecting' | 'connected'
  let busy = $state(false)

  const status = useStatus()
  const speed = useSpeedStream()

  function powerStateFor(s: Status): PowerState {
    if (busy) return 'connecting'
    return s.connected ? 'connected' : 'disconnected'
  }

  async function toggle(connected: boolean) {
    busy = true
    try {
      if (!connected) {
        if (platform.name === 'native') {
          const perm = await platform.requestVpnPermission()
          // 'denied' halts the flow; 'granted' and 'unsupported' both fall
          // through to engineStart. 'unsupported' means the native binary
          // doesn't yet expose requestPermission — Phase 1's graceful-
          // degradation contract (spec §7.5/§7.7) wants us to try REST.
          if (perm === 'denied') {
            toasts.error('VPN permission denied')
            busy = false
            return
          }
        }
        await platform.engineStart()
      } else {
        await platform.engineStop()
      }
      invalidate('dashboard.status')
    } catch (e) {
      toasts.error(errorMessage(e))
    } finally {
      busy = false
    }
  }

  function formatUptime(s: number): string {
    if (s < 60) return `${s}s`
    const m = Math.floor(s / 60)
    if (m < 60) return `${m}m`
    const h = Math.floor(m / 60)
    return `${h}h ${m % 60}m`
  }

  function formatSpeed(bps: number): string {
    if (bps >= 1e6) return `${(bps / 1e6).toFixed(1)} MB/s`
    if (bps >= 1e3) return `${(bps / 1e3).toFixed(1)} KB/s`
    return `${bps} B/s`
  }
</script>

<AsyncBoundary resource={status}>
  {#snippet children(s: Status)}
    {@const ps = powerStateFor(s)}
    <div class="page">
      <div class="label" data-state={ps}>
        {#if ps === 'connected'}
          {t('now.connected')} · {formatUptime(s.uptime ?? 0)}
        {:else if ps === 'connecting'}
          {t('now.connecting')}
        {:else}
          {t('now.disconnected')}
        {/if}
      </div>

      <PowerButton
        state={ps}
        labels={{
          connect: t('now.connect'),
          disconnect: t('now.disconnect'),
          connecting: t('now.connecting'),
        }}
        onToggle={() => toggle(!!s.connected)}
      />

      {#if ps === 'connected'}
        <div class="speeds">
          <span>↓ {formatSpeed(speed.data?.download ?? 0)}</span>
          <span>↑ {formatSpeed(speed.data?.upload ?? 0)}</span>
        </div>
      {/if}

      <ServerChip
        serverName={s.server?.name ?? s.server?.addr ?? '—'}
        transport={s.transport ?? ''}
        state={ps}
        onClick={() => navigate('/servers')}
      />

      <button class="switch-link" onclick={() => navigate('/servers')}>
        {t('now.switchServer')} →
      </button>
    </div>
  {/snippet}
</AsyncBoundary>

<style>
  .page {
    display: flex; flex-direction: column; align-items: center;
    gap: var(--shuttle-space-4);
    padding: var(--shuttle-space-5);
    max-width: 420px; margin: 0 auto;
    min-height: 70vh; justify-content: center;
  }
  .label {
    font-size: var(--shuttle-text-xs);
    text-transform: uppercase; letter-spacing: 0.1em;
    color: var(--shuttle-fg-muted);
  }
  .label[data-state="connected"]  { color: var(--shuttle-success, #3fb950); }
  .label[data-state="connecting"] { color: var(--shuttle-warning, #d29922); }
  .speeds {
    display: flex; gap: var(--shuttle-space-5);
    font-size: var(--shuttle-text-sm);
    color: var(--shuttle-fg-secondary);
    font-variant-numeric: tabular-nums;
  }
  .switch-link {
    background: transparent; border: 0; cursor: pointer;
    color: var(--shuttle-accent);
    font-size: var(--shuttle-text-sm);
    padding: var(--shuttle-space-2) var(--shuttle-space-3);
    min-height: 44px;
  }
</style>
