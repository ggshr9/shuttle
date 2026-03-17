<script lang="ts">
  import { t } from '../i18n/index'

  let {
    config = $bindable(),
    geoStatus,
    updatingGeo,
    onUpdateGeoData,
  } = $props()
</script>

<section>
  <h3>{t('settings.geodata')}</h3>
  <div class="system-proxy-row" style="margin-top: 0; padding-top: 0; border-top: none;">
    <label class="system-proxy-label">
      <input type="checkbox" bind:checked={config.routing.geodata.enabled} />
      <span class="label-text">{t('settings.geodataEnabled')}</span>
      <span class="hint">{t('settings.geodataHint')}</span>
    </label>
  </div>
  {#if config.routing.geodata.enabled}
    <div class="geo-status">
      {#if geoStatus}
        <div class="geo-info">
          <span class="geo-label">{t('settings.geodataFiles')}</span>
          <span class="geo-value">{geoStatus.files_present?.length || 0} / 6</span>
        </div>
        {#if geoStatus.last_update && geoStatus.last_update !== '0001-01-01T00:00:00Z'}
          <div class="geo-info">
            <span class="geo-label">{t('settings.geodataLastUpdate')}</span>
            <span class="geo-value">{new Date(geoStatus.last_update).toLocaleString()}</span>
          </div>
        {:else}
          <div class="geo-info">
            <span class="geo-label">{t('settings.geodataLastUpdate')}</span>
            <span class="geo-value geo-warn">{t('settings.geodataNever')}</span>
          </div>
        {/if}
        {#if geoStatus.last_error}
          <div class="geo-info">
            <span class="geo-label geo-error">{t('settings.geodataError')}</span>
            <span class="geo-value geo-error">{geoStatus.last_error}</span>
          </div>
        {/if}
      {/if}
      <button class="geo-update-btn" onclick={onUpdateGeoData} disabled={updatingGeo}>
        {updatingGeo ? t('settings.geodataUpdating') : t('settings.geodataUpdateNow')}
      </button>
      <div class="geo-info" style="margin-top: 8px;">
        <label>
          <input type="checkbox" bind:checked={config.routing.geodata.auto_update} />
          {t('settings.geodataAutoUpdate')}
        </label>
      </div>
    </div>
  {/if}
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

  .system-proxy-row {
    margin-top: 12px;
    padding-top: 12px;
    border-top: 1px solid var(--border);
  }

  .system-proxy-label {
    display: flex;
    align-items: center;
    gap: 8px;
    cursor: pointer;
  }

  .system-proxy-label .label-text {
    font-size: 13px;
    color: var(--text-primary);
  }

  .system-proxy-label .hint {
    font-size: 11px;
    color: var(--text-secondary);
    margin-left: auto;
  }

  .geo-status {
    margin-top: 12px;
    padding: 12px;
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 8px;
  }

  .geo-info {
    display: flex;
    justify-content: space-between;
    align-items: center;
    font-size: 13px;
    padding: 4px 0;
  }

  .geo-label { color: var(--text-secondary); }
  .geo-value { color: var(--text-primary); }
  .geo-warn { color: #d29922; }
  .geo-error { color: var(--accent-red); font-size: 12px; }

  .geo-update-btn {
    margin-top: 12px;
    width: 100%;
    padding: 8px;
    background: var(--bg-tertiary);
    color: var(--accent);
    border: 1px solid var(--border);
    border-radius: 6px;
    cursor: pointer;
    font-size: 13px;
  }
  .geo-update-btn:hover { background: #30363d; }
  .geo-update-btn:disabled { opacity: 0.5; cursor: default; }
</style>
