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
    border-radius: 8px;
    padding: 12px 16px;
    margin-bottom: 12px;
    margin-top: 24px;
  }

  h3 { font-size: 14px; color: var(--text-secondary); margin: 20px 0 10px; }

  .export-buttons {
    display: flex;
    gap: 12px;
  }

  .export-btn {
    background: var(--bg-tertiary);
    color: var(--accent);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px 16px;
    text-decoration: none;
    font-size: 13px;
    transition: background 0.2s;
  }

  .export-btn:hover {
    background: #30363d;
  }
</style>
