<script lang="ts">
  import { t } from '../i18n/index'

  let { config = $bindable() } = $props()
</script>

<section>
  <h3>{t('settings.log')}</h3>
  <label class="row">
    <span>{t('settings.logLevel')}</span>
    <select bind:value={config.log.level}>
      <option value="debug">{t('settings.logDebug')}</option>
      <option value="info">{t('settings.logInfo')}</option>
      <option value="warn">{t('settings.logWarn')}</option>
      <option value="error">{t('settings.logError')}</option>
    </select>
  </label>
</section>

<style>
  section {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 12px 16px;
    margin-bottom: 12px;
  }

  h3 { font-size: 14px; color: var(--text-secondary); margin: 20px 0 10px; }

  .row {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 6px 0;
  }

  .row span { font-size: 13px; color: var(--text-secondary); min-width: 100px; }

  select {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 6px 10px;
    color: var(--text-primary);
    font-size: 13px;
  }
</style>
