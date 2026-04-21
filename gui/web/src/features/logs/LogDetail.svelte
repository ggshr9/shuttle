<script lang="ts">
  import { Icon, Badge } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { formatBytes, formatDuration, formatTimestamp } from '@/lib/format'
  import { logsStore } from './store.svelte'

  const entry = $derived(logsStore.selected)
</script>

<section class="detail">
  {#if !entry}
    <div class="placeholder">
      <Icon name="logs" size={20} />
      <p>Select a log entry to inspect</p>
    </div>
  {:else}
    <header>
      <Badge variant={entry.level === 'error' ? 'danger' : entry.level === 'warn' ? 'warning' : 'neutral'}>
        {entry.level.toUpperCase()}
      </Badge>
      <span class="time">{formatTimestamp(entry.time)}</span>
    </header>

    <p class="msg">{entry.msg}</p>

    {#if entry.details}
      {@const d = entry.details}
      <dl>
        <dt>{t('logs.target')}</dt>
        <dd class="mono">{d.target}</dd>
        <dt>{t('logs.protocol')}</dt>
        <dd>{d.protocol.toUpperCase()}</dd>
        <dt>{t('logs.rule')}</dt>
        <dd>{d.rule}</dd>
        {#if d.process}
          <dt>{t('logs.process')}</dt>
          <dd class="mono">{d.process}</dd>
        {/if}
        {#if d.state === 'closed'}
          <dt>{t('logs.duration')}</dt>
          <dd>{formatDuration(d.duration)}</dd>
          <dt>{t('logs.traffic')}</dt>
          <dd>
            <span class="in">↓ {formatBytes(d.bytesIn)}</span>
            <span class="out">↑ {formatBytes(d.bytesOut)}</span>
          </dd>
        {/if}
      </dl>
    {/if}
  {/if}
</section>

<style>
  .detail {
    padding: var(--shuttle-space-5);
    border-left: 1px solid var(--shuttle-border);
    background: var(--shuttle-bg-subtle);
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: var(--shuttle-space-4);
  }
  .placeholder {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--shuttle-space-3);
    height: 100%;
    color: var(--shuttle-fg-muted);
    font-size: var(--shuttle-text-sm);
  }
  .placeholder p { margin: 0; }
  header { display: flex; align-items: center; gap: var(--shuttle-space-3); }
  .time {
    font-family: var(--shuttle-font-mono);
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-muted);
  }
  .msg {
    margin: 0;
    padding: var(--shuttle-space-3);
    background: var(--shuttle-bg-surface);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    font-family: var(--shuttle-font-mono);
    font-size: var(--shuttle-text-sm);
    color: var(--shuttle-fg-primary);
    word-break: break-word;
  }
  dl {
    display: grid;
    grid-template-columns: 96px 1fr;
    row-gap: var(--shuttle-space-2);
    column-gap: var(--shuttle-space-3);
    margin: 0;
    font-size: var(--shuttle-text-sm);
  }
  dt { color: var(--shuttle-fg-muted); }
  dd { margin: 0; color: var(--shuttle-fg-primary); }
  dd.mono { font-family: var(--shuttle-font-mono); font-size: var(--shuttle-text-xs); }
  dd .in  { color: var(--shuttle-success); margin-right: var(--shuttle-space-3); }
  dd .out { color: var(--shuttle-accent); }
</style>
