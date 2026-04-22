<script lang="ts">
  import { onMount } from 'svelte'
  import { AsyncBoundary, Card, Section, StatRow, Tabs, Spinner } from '@/ui'
  import { useRoute, navigate } from '@/lib/router'
  import { t } from '@/lib/i18n/index'
  import { useStatus, usePeers } from './resource.svelte'
  import PeerTable from './PeerTable.svelte'
  import type { Component } from 'svelte'

  const statusRes = useStatus()
  const peersRes = usePeers()

  const route = useRoute()
  const tab = $derived(route.query.tab === 'topology' ? 'topology' : 'peers')

  function setTab(v: string) {
    const q = new URLSearchParams({ ...route.query })
    if (v === 'topology') q.set('tab', 'topology')
    else q.delete('tab')
    const qs = q.toString()
    navigate('/mesh' + (qs ? '?' + qs : ''), { replace: true })
  }

  // Lazy-load TopologyChart only when the Topology tab is visited. Keeps the
  // initial Mesh bundle smaller for users who only want the peer list.
  let TopologyChart = $state<Component<{ peers: unknown[]; selfIP?: string }> | null>(null)
  $effect(() => {
    if (tab === 'topology' && !TopologyChart) {
      import('./TopologyChart.svelte').then((mod) => {
        TopologyChart = mod.default as Component<{ peers: unknown[]; selfIP?: string }>
      }).catch(() => {})
    }
  })
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

        <Tabs
          items={[
            { value: 'peers',    label: t('mesh.tab.peers') },
            { value: 'topology', label: t('mesh.tab.topology') },
          ]}
          value={tab}
          onValueChange={setTab}
        />

        {#if tab === 'peers'}
          <AsyncBoundary resource={peersRes}>
            {#snippet children(peers)}
              <PeerTable {peers} />
            {/snippet}
          </AsyncBoundary>
        {:else}
          {#if TopologyChart}
            <TopologyChart peers={peersRes.data ?? []} selfIP={status.virtual_ip} />
          {:else}
            <div class="loading"><Spinner size={18} /></div>
          {/if}
        {/if}
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
  .summary {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: var(--shuttle-space-3);
    margin-bottom: var(--shuttle-space-4);
  }
  .loading {
    display: flex;
    justify-content: center;
    padding: var(--shuttle-space-6);
  }
  @media (max-width: 720px) {
    .summary { grid-template-columns: 1fr; }
  }
</style>
