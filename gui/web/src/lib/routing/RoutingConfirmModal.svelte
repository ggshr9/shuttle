<script lang="ts">
  import { t } from '../i18n/index'

  let {
    confirmTemplate = $bindable(),
    applyingTemplate,
    onApply,
  } = $props()
</script>

{#if confirmTemplate}
<div class="overlay" onclick={() => (confirmTemplate = null)} role="dialog" aria-modal="true" aria-labelledby="confirm-template-title" onkeydown={(e) => e.key === 'Escape' && (confirmTemplate = null)}>
  <div class="modal" onclick={(e) => e.stopPropagation()}>
    <h3 id="confirm-template-title">{t('routing.confirmTemplate')}</h3>
    <p class="modal-hint">{t('routing.confirmTemplateMsg')}</p>
    <div class="modal-actions">
      <button class="close-btn" onclick={() => (confirmTemplate = null)}>{t('common.cancel')}</button>
      <button class="apply-btn" onclick={() => onApply(confirmTemplate)} disabled={applyingTemplate}>
        {applyingTemplate ? t('routing.saving') : t('routing.replace')}
      </button>
    </div>
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

  .modal-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 12px;
  }

  .close-btn {
    background: var(--bg-tertiary);
    color: var(--text-primary);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px;
    cursor: pointer;
    font-size: 13px;
  }

  .apply-btn {
    background: var(--btn-bg);
    color: #fff;
    border: none;
    border-radius: 6px;
    padding: 8px 16px;
    cursor: pointer;
    font-size: 13px;
  }

  .apply-btn:hover { background: var(--btn-bg-hover); }
  .apply-btn:disabled { opacity: 0.5; }
</style>
