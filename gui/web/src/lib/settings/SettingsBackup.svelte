<script lang="ts">
  import { t } from '../i18n/index'
  import { api } from '../api'

  let {
    onMessage,
  } = $props()

  let restoring = $state(false)
  let fileInput = $state(null)

  function triggerRestore() {
    fileInput?.click()
  }

  async function handleRestore(e) {
    const file = e.target.files?.[0]
    if (!file) return

    restoring = true
    try {
      const text = await file.text()
      const backup = JSON.parse(text)
      const res = await api.restore(backup)
      onMessage(t('settings.restored', { servers: res.servers, subscriptions: res.subscriptions }), 'success')
    } catch (err) {
      onMessage(t('settings.restoreFailed') + ': ' + (err.message || err), 'error')
    } finally {
      restoring = false
      e.target.value = ''
    }
  }
</script>

<section class="backup-section">
  <h3>{t('settings.backup')}</h3>
  <p class="section-hint">{t('settings.backupHint')}</p>
  <div class="backup-buttons">
    <a href={api.backupUrl()} download="shuttle-backup.json" class="backup-btn">
      <span class="btn-icon">📦</span>
      {t('settings.createBackup')}
    </a>
    <button class="backup-btn restore" onclick={triggerRestore} disabled={restoring}>
      <span class="btn-icon">📥</span>
      {restoring ? t('settings.restoring') : t('settings.restoreBackup')}
    </button>
    <input
      type="file"
      accept=".json"
      bind:this={fileInput}
      onchange={handleRestore}
      style="display: none"
    />
  </div>
</section>

<style>
  .backup-section {
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

  .backup-buttons {
    display: flex;
    gap: 12px;
    flex-wrap: wrap;
  }

  .backup-btn {
    display: flex;
    align-items: center;
    gap: 8px;
    background: var(--bg-tertiary);
    color: var(--text-primary);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 10px 16px;
    text-decoration: none;
    font-size: 13px;
    cursor: pointer;
    transition: all 0.2s;
  }

  .backup-btn:hover {
    background: #30363d;
    border-color: var(--accent-green);
  }

  .backup-btn.restore {
    background: var(--bg-secondary);
    border-color: var(--accent);
    color: var(--accent);
  }

  .backup-btn.restore:hover {
    background: rgba(88, 166, 255, 0.1);
  }

  .backup-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .btn-icon {
    font-size: 16px;
  }
</style>
