<script lang="ts">
  import { Button, Icon, Badge } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { connectPeer } from './resource.svelte'
  import type { MeshPeer } from '@/lib/api/types'

  interface Props { peer: MeshPeer }
  let { peer }: Props = $props()

  let busy = $state(false)

  async function doConnect() {
    busy = true
    try { await connectPeer(peer.virtual_ip) } finally { busy = false }
  }

  const stateClass = $derived(
    peer.state === 'connected' ? 'ok'
      : peer.state === 'connecting' ? 'warn'
      : peer.state === 'failed' ? 'bad'
      : 'unknown'
  )

  const stateVariant = $derived<'success' | 'warning' | 'danger' | 'neutral'>(
    peer.state === 'connected' ? 'success'
      : peer.state === 'connecting' ? 'warning'
      : peer.state === 'failed' ? 'danger'
      : 'neutral'
  )
</script>

<div class="row">
  <span class={`dot ${stateClass}`}></span>
  <span class="vip">{peer.virtual_ip}</span>
  <Badge variant={stateVariant}>{peer.state}</Badge>
  <span class="method">{peer.method ?? '—'}</span>
  <span class="rtt">{peer.avg_rtt_ms != null ? `${peer.avg_rtt_ms} ms` : '—'}</span>
  <span class="loss">{peer.packet_loss != null ? `${(peer.packet_loss * 100).toFixed(1)} %` : '—'}</span>
  <span class="score">{peer.score != null ? peer.score.toFixed(0) : '—'}</span>
  <span class="action">
    {#if peer.state !== 'connected'}
      <Button size="sm" variant="ghost" loading={busy} onclick={doConnect}>
        <Icon name="check" size={14} title={t('mesh.connect')} />
      </Button>
    {/if}
  </span>
</div>

<style>
  .row {
    display: grid;
    grid-template-columns: 16px 160px 90px 80px 80px 80px 60px auto;
    align-items: center;
    gap: var(--shuttle-space-3);
    height: 48px;
    padding: 0 var(--shuttle-space-4);
    border-top: 1px solid var(--shuttle-border);
    font-size: var(--shuttle-text-sm);
  }
  .row:first-child { border-top: 0; }
  .dot {
    width: 8px; height: 8px; border-radius: 50%;
    background: var(--shuttle-fg-muted);
  }
  .dot.ok   { background: var(--shuttle-success); }
  .dot.warn { background: var(--shuttle-warning); }
  .dot.bad  { background: var(--shuttle-danger); }
  .vip {
    font-family: var(--shuttle-font-mono);
    color: var(--shuttle-fg-primary);
    font-weight: var(--shuttle-weight-medium);
  }
  .method, .rtt, .loss, .score {
    font-family: var(--shuttle-font-mono);
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-secondary);
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
  .action { justify-self: end; }
</style>
