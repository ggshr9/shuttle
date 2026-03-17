<script lang="ts">
  import { t } from '../i18n/index'

  let {
    show = $bindable(),
    processes,
    onAddPerApp,
  } = $props()
</script>

{#if show}
<div class="overlay" onclick={() => (show = false)} role="dialog" aria-modal="true" aria-labelledby="perapp-picker-title" onkeydown={(e) => e.key === 'Escape' && (show = false)}>
  <div class="picker-modal" onclick={(e) => e.stopPropagation()}>
    <h3 id="perapp-picker-title">{t('settings.selectProcess')}</h3>
    <p class="picker-hint">{t('settings.selectProcessHint')}</p>
    {#if processes.length}
      <div class="picker-list">
        {#each processes as proc}
          <button class="picker-item" onclick={() => { onAddPerApp(proc.name); }}>
            <span class="picker-name">{proc.name}</span>
            <span class="picker-conns">{proc.conns} conn{proc.conns !== 1 ? 's' : ''}</span>
          </button>
        {/each}
      </div>
    {:else}
      <p class="picker-empty">{t('settings.noProcesses')}</p>
    {/if}
    <button class="picker-close" onclick={() => (show = false)}>{t('settings.done')}</button>
  </div>
</div>
{/if}

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
  }

  .picker-modal {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 20px;
    width: 400px;
    max-height: 500px;
    display: flex;
    flex-direction: column;
  }

  .picker-modal h3 { font-size: 16px; margin: 0 0 4px; color: var(--text-primary); }
  .picker-hint { font-size: 12px; color: var(--text-muted); margin: 0 0 12px; }

  .picker-list {
    overflow-y: auto;
    max-height: 350px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .picker-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px 12px;
    cursor: pointer;
    color: var(--text-primary);
    font-size: 13px;
    width: 100%;
    text-align: left;
  }

  .picker-item:hover { border-color: var(--accent); }
  .picker-name { font-weight: 500; }
  .picker-conns { font-size: 11px; color: var(--text-muted); }
  .picker-empty { font-size: 13px; color: var(--text-muted); }

  .picker-close {
    margin-top: 12px;
    background: var(--bg-tertiary);
    color: var(--text-primary);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px;
    cursor: pointer;
    font-size: 13px;
  }
</style>
