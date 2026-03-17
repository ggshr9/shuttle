<script lang="ts">
  import { t } from '../i18n/index'

  let { config = $bindable() } = $props()
</script>

<section>
  <h3>{t('settings.dns')}</h3>
  <label class="row">
    <span>{t('settings.domesticDns')}</span>
    <input bind:value={config.routing.dns.domestic} placeholder="223.5.5.5" />
  </label>
  <label class="row">
    <span>{t('settings.remoteDns')}</span>
    <input bind:value={config.routing.dns.remote.server} placeholder="https://1.1.1.1/dns-query" />
  </label>
  <label class="row">
    <span>{t('settings.remoteVia')}</span>
    <select bind:value={config.routing.dns.remote.via}>
      <option value="proxy">{t('routing.proxy')}</option>
      <option value="direct">{t('routing.direct')}</option>
    </select>
  </label>
  <div class="checkbox-row">
    <label>
      <input type="checkbox" bind:checked={config.routing.dns.cache} />
      {t('settings.enableDnsCache')}
    </label>
    <label>
      <input type="checkbox" bind:checked={config.routing.dns.prefetch} />
      {t('settings.enableDnsPrefetch')}
    </label>
  </div>
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

  .row input[type="text"], .row input:not([type]) {
    flex: 1;
  }

  .checkbox-row {
    display: flex;
    gap: 16px;
    margin-top: 8px;
  }

  .checkbox-row label {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
    color: var(--text-secondary);
  }

  input[type="text"], input:not([type]) {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 6px 10px;
    color: var(--text-primary);
    font-size: 13px;
  }

  select {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 6px 10px;
    color: var(--text-primary);
    font-size: 13px;
  }
</style>
