<script lang="ts">
  import { t } from '../i18n/index'
  import { api } from '../api'
</script>

<section class="export-section">
  <h3>{t('settings.export')}</h3>
  <div class="export-buttons">
    <a href={api.exportConfig('json')} download class="export-btn">
      {t('settings.exportJson')}
    </a>
    <a href={api.exportConfig('uri')} download class="export-btn">
      {t('settings.exportUri')}
    </a>
  </div>
</section>

<style>
  .export-section {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: 16px 20px;
    margin-bottom: 12px;
    margin-top: 24px;
  }

  h3 { font-size: 14px; font-weight: 600; color: var(--text-primary); margin: 20px 0 10px; }

  .export-buttons {
    display: flex;
    gap: 12px;
  }

  .export-btn {
    background: var(--bg-tertiary);
    color: var(--accent);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 8px 16px;
    text-decoration: none;
    font-size: 13px;
    transition: background 0.2s;
  }

  .export-btn:hover {
    background: var(--bg-hover);
  }
</style>
