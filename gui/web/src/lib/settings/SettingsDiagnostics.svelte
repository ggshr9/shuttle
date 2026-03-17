<script lang="ts">
  import { t } from '../i18n/index'
  import { api } from '../api'

  let { onMessage } = $props()

  let downloadingDiag = $state(false)

  async function downloadDiagnostics() {
    downloadingDiag = true
    try {
      await api.downloadDiagnostics()
    } catch (err) {
      onMessage(t('settings.diagnostics') + ' failed: ' + (err.message || err), 'error')
    } finally {
      downloadingDiag = false
    }
  }
</script>

<section class="diagnostics-section">
  <h3>{t('settings.diagnostics')}</h3>
  <p class="section-hint">{t('settings.diagnosticsDesc')}</p>
  <button class="diag-btn" onclick={downloadDiagnostics} disabled={downloadingDiag}>
    {downloadingDiag ? t('settings.downloading') : t('settings.downloadDiagnostics')}
  </button>
</section>

<style>
  .diagnostics-section {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 12px 16px;
    margin-bottom: 12px;
    margin-top: 24px;
  }

  h3 { font-size: 14px; color: var(--text-secondary); margin: 20px 0 10px; }

  .section-hint {
    font-size: 12px;
    color: #6e7681;
    margin: 4px 0 12px;
  }

  .diag-btn {
    width: 100%;
    padding: 10px;
    background: var(--bg-tertiary);
    color: var(--accent);
    border: 1px solid var(--border);
    border-radius: 6px;
    cursor: pointer;
    font-size: 13px;
    transition: background 0.2s;
  }

  .diag-btn:hover:not(:disabled) {
    background: #30363d;
  }

  .diag-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
</style>
