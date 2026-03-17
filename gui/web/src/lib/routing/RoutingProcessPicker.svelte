<script lang="ts">
  import { t } from '../i18n/index'

  let {
    show = $bindable(),
    processes,
    onSelectProcess,
  } = $props()

  function close() {
    show = false
  }
</script>

{#if show}
<div class="overlay" onclick={close} role="dialog" aria-modal="true" aria-labelledby="process-picker-dialog-title" onkeydown={(e) => e.key === 'Escape' && close()}>
  <div class="picker" onclick={(e) => e.stopPropagation()}>
    <h3 id="process-picker-dialog-title">{t('routing.selectProcess')}</h3>
    <p class="picker-hint">{t('routing.selectProcessHint')}</p>
    {#if processes.length}
      <div class="proc-list">
        {#each processes as proc}
          <button class="proc-item" onclick={() => onSelectProcess(proc.name)}>
            <span class="proc-name">{proc.name}</span>
            <span class="proc-conns">{proc.conns} conn{proc.conns !== 1 ? 's' : ''}</span>
          </button>
        {/each}
      </div>
    {:else}
      <p class="empty">{t('routing.noProcesses')}</p>
    {/if}
    <button class="close-btn" onclick={close}>{t('routing.done')}</button>
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

  .picker {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 20px;
    width: 400px;
    max-height: 500px;
    display: flex;
    flex-direction: column;
  }

  .picker h3 { font-size: 16px; margin: 0 0 4px; color: var(--text-primary); }
  .picker-hint { font-size: 12px; color: var(--text-muted); margin: 0 0 12px; }

  .proc-list {
    overflow-y: auto;
    max-height: 350px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .proc-item {
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

  .proc-item:hover { border-color: var(--accent); }
  .proc-name { font-weight: 500; }
  .proc-conns { font-size: 11px; color: var(--text-muted); }

  .close-btn {
    margin-top: 12px;
    background: var(--bg-tertiary);
    color: var(--text-primary);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px;
    cursor: pointer;
    font-size: 13px;
  }

  .empty { font-size: 13px; color: var(--text-muted); }
</style>
