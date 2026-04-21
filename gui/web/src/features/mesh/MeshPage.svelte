<script lang="ts">
  import { AsyncBoundary, Card, Section, StatRow, Icon, Tooltip } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { useStatus, usePeers } from './resource.svelte'
  import TopologyChart from './TopologyChart.svelte'
  import PeerTable from './PeerTable.svelte'

  const statusRes = useStatus()
  const peersRes = usePeers()
</script>

<Section
  title={t('nav.mesh')}
  description={statusRes.data?.enabled ? t('mesh.count', { n: peersRes.data?.length ?? 0 }) : undefined}
>
  <AsyncBoundary resource={statusRes}>
    {#snippet children(status)}
      {#if !status.enabled}
        <Card>
          <div class="disabled">
            <h3>{t('mesh.disabled.title')}</h3>
            <p>{t('mesh.disabled.desc')}</p>
          </div>
        </Card>
      {:else}
        <Card>
          <div class="summary">
            <StatRow label={t('mesh.virtualIp')} value={status.virtual_ip ?? '—'} mono />
            <StatRow label={t('mesh.cidr')}      value={status.cidr ?? '—'} mono />
            <StatRow label={t('mesh.peerCount')} value={String(status.peer_count ?? 0)} />
          </div>
        </Card>

        <h3 class="section-head">
          <span>{t('mesh.topology')}</span>
          <Tooltip content={t('mesh.topologyTooltip')}>
            <Icon name="info" size={12} />
          </Tooltip>
        </h3>
        <TopologyChart peers={peersRes.data ?? []} selfIP={status.virtual_ip} />

        <h3 class="section-head">
          <span>{t('mesh.peers')}</span>
        </h3>
        <AsyncBoundary resource={peersRes}>
          {#snippet children(peers)}
            <PeerTable {peers} />
          {/snippet}
        </AsyncBoundary>
      {/if}
    {/snippet}
  </AsyncBoundary>
</Section>

<style>
  .disabled { text-align: center; padding: var(--shuttle-space-5); }
  .disabled h3 {
    margin: 0 0 var(--shuttle-space-2);
    font-size: var(--shuttle-text-base);
    color: var(--shuttle-fg-primary);
  }
  .disabled p {
    margin: 0;
    font-size: var(--shuttle-text-sm);
    color: var(--shuttle-fg-muted);
  }
  .summary { display: grid; grid-template-columns: repeat(3, 1fr); gap: var(--shuttle-space-3); }
  .section-head {
    display: flex; align-items: center; gap: var(--shuttle-space-2);
    margin: var(--shuttle-space-5) 0 var(--shuttle-space-3);
    font-size: var(--shuttle-text-sm);
    font-weight: var(--shuttle-weight-semibold);
    color: var(--shuttle-fg-primary);
  }
  .section-head :global(button) { color: var(--shuttle-fg-muted); }
</style>
