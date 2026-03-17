<script lang="ts">
  import { t } from '../i18n/index'

  let { config = $bindable() } = $props()
</script>

<section>
  <h3>{t('settings.meshVPN')}</h3>
  <div class="system-proxy-row" style="margin-top: 0; padding-top: 0; border-top: none;">
    <label class="system-proxy-label">
      <input type="checkbox" bind:checked={config.mesh.enabled} />
      <span class="label-text">{t('settings.meshEnabled')}</span>
      <span class="hint">{t('settings.meshEnabledHint')}</span>
    </label>
  </div>
  {#if config.mesh.enabled}
    <div class="system-proxy-row">
      <label class="system-proxy-label">
        <input type="checkbox" bind:checked={config.mesh.p2p_enabled} />
        <span class="label-text">{t('settings.meshP2P')}</span>
        <span class="hint">{t('settings.meshP2PHint')}</span>
      </label>
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
</style>
