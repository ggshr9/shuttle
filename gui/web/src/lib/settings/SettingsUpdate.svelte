<script lang="ts">
  import { t } from '../i18n/index'

  let {
    currentVersion,
    updateInfo,
    checkingUpdate,
    onCheckForUpdate,
  } = $props()

  let showChangelog = $state(false)

  function formatSize(bytes) {
    if (!bytes) return ''
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
    return (bytes / 1024 / 1024).toFixed(1) + ' MB'
  }
</script>

<section class="update-section">
  <h3>{t('settings.updates')}</h3>
  <div class="version-info">
    <span class="version-label">{t('settings.currentVersion')}:</span>
    <span class="version-value">{currentVersion}</span>
  </div>

  {#if updateInfo?.available}
    <div class="update-available">
      <div class="update-badge">
        <span class="badge-icon">🎉</span>
        {t('settings.newVersion')}: {updateInfo.latest_version}
      </div>
      <div class="update-actions">
        <button class="changelog-btn" onclick={() => (showChangelog = !showChangelog)}>
          {showChangelog ? t('settings.hideChangelog') : t('settings.showChangelog')}
        </button>
        <a
          href={updateInfo.release_url}
          target="_blank"
          rel="noopener noreferrer"
          class="download-btn"
        >
          {t('settings.viewRelease')}
        </a>
        {#if updateInfo.download_url}
          <a
            href={updateInfo.download_url}
            class="download-btn primary"
          >
            {t('settings.download')} ({formatSize(updateInfo.asset_size)})
          </a>
        {/if}
      </div>
      {#if showChangelog && updateInfo.changelog}
        <div class="changelog">
          <pre>{updateInfo.changelog}</pre>
        </div>
      {/if}
    </div>
  {:else if updateInfo}
    <p class="up-to-date">✓ {t('settings.upToDate')}</p>
  {/if}

  <button
    class="check-btn"
    onclick={() => onCheckForUpdate(true)}
    disabled={checkingUpdate}
  >
    {checkingUpdate ? t('settings.checking') : t('settings.checkUpdates')}
  </button>
</section>

<style>
  .update-section {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 12px 16px;
    margin-bottom: 12px;
    margin-top: 24px;
  }

  h3 { font-size: 14px; color: var(--text-secondary); margin: 20px 0 10px; }

  .version-info {
    display: flex;
    gap: 8px;
    margin-bottom: 12px;
  }

  .version-label {
    color: var(--text-secondary);
    font-size: 13px;
  }

  .version-value {
    color: var(--text-primary);
    font-size: 13px;
    font-family: 'Cascadia Code', 'Fira Code', monospace;
  }

  .update-available {
    background: rgba(63, 185, 80, 0.08);
    border: 1px solid var(--accent-green);
    border-radius: 8px;
    padding: 12px;
    margin-bottom: 12px;
  }

  .update-badge {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--accent-green);
    font-size: 14px;
    font-weight: 500;
    margin-bottom: 12px;
  }

  .badge-icon {
    font-size: 18px;
  }

  .update-actions {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }

  .changelog-btn {
    background: transparent;
    border: 1px solid var(--accent-green);
    color: var(--accent-green);
    border-radius: 6px;
    padding: 6px 12px;
    cursor: pointer;
    font-size: 12px;
  }

  .download-btn {
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
    color: var(--accent);
    border-radius: 6px;
    padding: 6px 12px;
    text-decoration: none;
    font-size: 12px;
    cursor: pointer;
  }

  .download-btn.primary {
    background: var(--btn-bg);
    border-color: var(--btn-bg);
    color: #fff;
  }

  .download-btn:hover {
    background: #30363d;
  }

  .download-btn.primary:hover {
    background: var(--btn-bg-hover);
  }

  .changelog {
    margin-top: 12px;
    background: var(--bg-surface);
    border: 1px solid var(--bg-tertiary);
    border-radius: 6px;
    padding: 12px;
    max-height: 200px;
    overflow-y: auto;
  }

  .changelog pre {
    margin: 0;
    font-size: 12px;
    color: #c9d1d9;
    white-space: pre-wrap;
    word-break: break-word;
  }

  .up-to-date {
    color: var(--accent-green);
    font-size: 13px;
    margin-bottom: 12px;
  }

  .check-btn {
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
    color: var(--text-primary);
    border-radius: 6px;
    padding: 8px 16px;
    cursor: pointer;
    font-size: 13px;
  }

  .check-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .check-btn:hover:not(:disabled) {
    background: #30363d;
  }
</style>
