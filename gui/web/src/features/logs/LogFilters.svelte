<script lang="ts">
  import { Select, Switch } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { logsStore } from './store.svelte'
  import type { LogLevel } from './types'

  const LEVELS: LogLevel[] = ['debug', 'info', 'warn', 'error']

  const protocolOptions = [
    { value: 'all', label: t('logs.filterAll') },
    { value: 'tcp', label: 'TCP' },
    { value: 'udp', label: 'UDP' },
  ]
  const actionOptions = [
    { value: 'all',    label: t('logs.filterAll') },
    { value: 'proxy',  label: t('routing.proxy') },
    { value: 'direct', label: t('routing.direct') },
  ]
</script>

<aside class="filters">
  <section>
    <h4>{t('logs.level')}</h4>
    <div class="chips">
      {#each LEVELS as level (level)}
        <button
          type="button"
          class="chip"
          class:active={logsStore.levels.has(level)}
          class:warn={level === 'warn'}
          class:error={level === 'error'}
          onclick={() => logsStore.toggleLevel(level)}
        >
          {t(`logs.${level}`)}
        </button>
      {/each}
    </div>
  </section>

  <section>
    <h4>{t('logs.protocol')}</h4>
    <Select bind:value={logsStore.protocol} options={protocolOptions} />
  </section>

  <section>
    <h4>{t('logs.action')}</h4>
    <Select bind:value={logsStore.action} options={actionOptions} />
  </section>

  <section class="toggles">
    <Switch bind:checked={logsStore.showConnections} label={t('logs.showConnections')} />
    <Switch bind:checked={logsStore.autoScroll}      label={t('logs.autoScroll')} />
  </section>
</aside>

<style>
  .filters {
    display: flex;
    flex-direction: column;
    gap: var(--shuttle-space-5);
    padding: var(--shuttle-space-4);
    border-right: 1px solid var(--shuttle-border);
    background: var(--shuttle-bg-subtle);
    overflow-y: auto;
  }
  section { display: flex; flex-direction: column; gap: var(--shuttle-space-2); }
  h4 {
    margin: 0;
    font-size: var(--shuttle-text-xs);
    font-weight: var(--shuttle-weight-semibold);
    color: var(--shuttle-fg-muted);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .chips { display: flex; flex-wrap: wrap; gap: var(--shuttle-space-2); }
  .chip {
    appearance: none;
    border: 1px solid var(--shuttle-border);
    background: var(--shuttle-bg-base);
    color: var(--shuttle-fg-muted);
    border-radius: var(--shuttle-radius-sm);
    padding: var(--shuttle-space-1) var(--shuttle-space-3);
    font-size: var(--shuttle-text-xs);
    font-weight: var(--shuttle-weight-medium);
    cursor: pointer;
    transition: color 120ms, background 120ms, border-color 120ms;
  }
  .chip:hover { color: var(--shuttle-fg-primary); }
  .chip.active {
    color: var(--shuttle-fg-primary);
    background: var(--shuttle-bg-surface);
    border-color: var(--shuttle-fg-primary);
  }
  .chip.warn.active  { color: var(--shuttle-warning); border-color: var(--shuttle-warning); }
  .chip.error.active { color: var(--shuttle-danger); border-color: var(--shuttle-danger); }
  .toggles { gap: var(--shuttle-space-3); }
</style>
