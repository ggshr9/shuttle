<script lang="ts">
  import { onMount } from 'svelte'
  import { Button, Input, Section } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { logsStore } from './store.svelte'
  import LogFilters from './LogFilters.svelte'
  import LogList from './LogList.svelte'
  import LogDetail from './LogDetail.svelte'

  onMount(() => logsStore.subscribe())

  function exportLogs(): void {
    const lines = logsStore.filtered.map((e) => {
      const time = new Date(e.time).toISOString()
      let s = `[${time}] [${e.level.toUpperCase()}] ${e.msg}`
      if (e.details) {
        s += `\n  target=${e.details.target}`
        s += `\n  protocol=${e.details.protocol}`
        s += `\n  rule=${e.details.rule}`
        if (e.details.process)  s += `\n  process=${e.details.process}`
        if (e.details.duration) s += `\n  duration_ms=${e.details.duration}`
        if (e.details.bytesIn || e.details.bytesOut) {
          s += `\n  bytes_in=${e.details.bytesIn ?? 0} bytes_out=${e.details.bytesOut ?? 0}`
        }
      }
      return s
    })
    const blob = new Blob([lines.join('\n')], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `shuttle-logs-${new Date().toISOString().slice(0, 10)}.txt`
    a.click()
    URL.revokeObjectURL(url)
  }

  const hiddenCount = $derived(logsStore.entries.length - logsStore.filtered.length)
</script>

<Section
  title={t('logs.title')}
  description={t('logs.activeConnections', { count: logsStore.activeConnectionCount })}
>
  {#snippet actions()}
    <Button variant="ghost" onclick={() => logsStore.clear()} disabled={logsStore.entries.length === 0}>
      {t('logs.clear')}
    </Button>
    <Button variant="ghost" onclick={exportLogs} disabled={logsStore.filtered.length === 0}>
      {t('logs.export')}
    </Button>
  {/snippet}

  <div class="toolbar">
    <div class="search">
      <Input
        placeholder={t('logs.searchPlaceholder')}
        bind:value={logsStore.text}
      />
    </div>
    {#if hiddenCount > 0}
      <span class="count">
        {t('logs.showing', { shown: logsStore.filtered.length, total: logsStore.entries.length })}
      </span>
    {/if}
  </div>

  <div class="grid">
    <LogFilters />
    <LogList />
    <LogDetail />
  </div>
</Section>

<style>
  .toolbar {
    display: flex;
    align-items: center;
    gap: var(--shuttle-space-3);
    margin-bottom: var(--shuttle-space-3);
  }
  .search { flex: 1; max-width: 360px; }
  .count {
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-muted);
  }

  .grid {
    display: grid;
    grid-template-columns: 200px 1fr 320px;
    height: calc(100vh - 220px);
    min-height: 420px;
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    overflow: hidden;
    background: var(--shuttle-bg-surface);
  }
  .grid > :global(*) { min-width: 0; }
</style>
