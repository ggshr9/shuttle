<script lang="ts">
  import { onMount } from 'svelte'
  import { useRoute, navigate } from '@/lib/router'
  import { Tabs, Section, Button, Input } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import SpeedSparkline from '@/features/dashboard/SpeedSparkline.svelte'
  import TransportBreakdown from '@/features/dashboard/TransportBreakdown.svelte'
  import { useTransportStats } from '@/features/dashboard/resource.svelte'
  import LogFilters from '@/features/logs/LogFilters.svelte'
  import LogList from '@/features/logs/LogList.svelte'
  import { logsStore } from '@/features/logs/store.svelte'

  const route = useRoute()
  const tab = $derived(route.query.tab === 'logs' ? 'logs' : 'overview')

  const transports = useTransportStats()

  onMount(() => logsStore.subscribe())

  function setTab(v: string) {
    const q = new URLSearchParams({ ...route.query })
    if (v === 'logs') q.set('tab', 'logs')
    else q.delete('tab')
    const qs = q.toString()
    navigate('/activity' + (qs ? '?' + qs : ''), { replace: true })
  }

  const hiddenCount = $derived(logsStore.entries.length - logsStore.filtered.length)
</script>

<Section title={t('nav.activity')}>
  <Tabs
    items={[
      { value: 'overview', label: t('activity.overview') },
      { value: 'logs',     label: t('activity.logs') },
    ]}
    value={tab}
    onValueChange={setTab}
  />

  {#if tab === 'overview'}
    <div class="panels">
      <div class="panel">
        <h3 class="panel-title">{t('activity.throughput')}</h3>
        <SpeedSparkline />
      </div>
      <div class="panel">
        <h3 class="panel-title">{t('activity.transports')}</h3>
        <TransportBreakdown transports={transports.data ?? []} />
      </div>
    </div>
  {:else}
    <div class="toolbar">
      <div class="search">
        <Input placeholder={t('logs.searchPlaceholder')} bind:value={logsStore.text} />
      </div>
      <Button variant="ghost" onclick={() => logsStore.clear()} disabled={logsStore.entries.length === 0}>
        {t('logs.clear')}
      </Button>
      {#if hiddenCount > 0}
        <span class="count">
          {t('logs.showing', { shown: logsStore.filtered.length, total: logsStore.entries.length })}
        </span>
      {/if}
    </div>

    <div class="logs-grid">
      <LogFilters />
      <LogList />
    </div>
  {/if}
</Section>

<style>
  .panels {
    display: flex; flex-direction: column;
    gap: var(--shuttle-space-5);
    margin-top: var(--shuttle-space-4);
  }
  .panel {
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    padding: var(--shuttle-space-4);
    background: var(--shuttle-bg-surface);
  }
  .panel-title {
    font-size: var(--shuttle-text-sm);
    font-weight: var(--shuttle-weight-semibold);
    color: var(--shuttle-fg-primary);
    margin: 0 0 var(--shuttle-space-3);
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }

  .toolbar {
    display: flex; align-items: center;
    gap: var(--shuttle-space-3);
    margin: var(--shuttle-space-4) 0 var(--shuttle-space-3);
  }
  .search { flex: 1; max-width: 360px; }
  .count {
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-muted);
  }

  .logs-grid {
    display: grid;
    grid-template-columns: 200px 1fr;
    height: calc(100vh - 260px);
    min-height: 420px;
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    overflow: hidden;
    background: var(--shuttle-bg-surface);
  }
  .logs-grid > :global(*) { min-width: 0; }

  @media (max-width: 720px) {
    .logs-grid {
      grid-template-columns: 1fr;
      height: auto;
      min-height: 50vh;
    }
  }
</style>
