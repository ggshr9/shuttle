<script lang="ts">
  import { t } from '../i18n/index'

  let {
    show = $bindable(),
    templates,
    applyingTemplate,
    onRequestApply,
  } = $props()
</script>

{#if show}
<div class="overlay" onclick={() => (show = false)} role="dialog" aria-modal="true" aria-labelledby="templates-dialog-title" onkeydown={(e) => e.key === 'Escape' && (show = false)}>
  <div class="modal" onclick={(e) => e.stopPropagation()}>
    <h3 id="templates-dialog-title">{t('routing.routingTemplates')}</h3>
    <p class="modal-hint">{t('routing.templateHint')}</p>
    <div class="template-list">
      {#each templates as t}
        <button class="template-item" onclick={() => onRequestApply(t.id)} disabled={applyingTemplate}>
          <span class="template-name">{t.name}</span>
          <span class="template-desc">{t.description}</span>
        </button>
      {/each}
    </div>
    <button class="close-btn" onclick={() => (show = false)}>{t('common.cancel')}</button>
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

  .modal {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 20px;
    width: 450px;
    max-height: 500px;
    display: flex;
    flex-direction: column;
  }

  .modal h3 { font-size: 16px; margin: 0 0 4px; color: var(--text-primary); }
  .modal-hint { font-size: 12px; color: var(--text-muted); margin: 0 0 12px; }

  .template-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
    overflow-y: auto;
    max-height: 300px;
  }

  .template-item {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 12px;
    cursor: pointer;
    color: var(--text-primary);
    text-align: left;
    width: 100%;
  }

  .template-item:hover { border-color: var(--accent-purple); }
  .template-item:disabled { opacity: 0.5; cursor: default; }

  .template-name { font-weight: 500; font-size: 14px; }
  .template-desc { font-size: 12px; color: var(--text-secondary); margin-top: 4px; }

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
</style>
