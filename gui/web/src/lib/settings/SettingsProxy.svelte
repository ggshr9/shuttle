<script lang="ts">
  import { t } from '../i18n/index'

  let {
    config = $bindable(),
    lanInfo,
    autostartEnabled,
    autostartLoading,
    newAppName = $bindable(),
    onLoadLanInfo,
    onToggleAutostart,
    onOpenPerAppPicker,
    onAddPerApp,
    onRemovePerApp,
  } = $props()
</script>

<section>
  <h3>{t('settings.proxyListeners')}</h3>
  <div class="grid">
    <label>
      <input type="checkbox" bind:checked={config.proxy.socks5.enabled} />
      SOCKS5
    </label>
    <input bind:value={config.proxy.socks5.listen} placeholder="127.0.0.1:1080" />

    <label>
      <input type="checkbox" bind:checked={config.proxy.http.enabled} />
      HTTP
    </label>
    <input bind:value={config.proxy.http.listen} placeholder="127.0.0.1:8080" />

    <label>
      <input type="checkbox" bind:checked={config.proxy.tun.enabled} />
      TUN
    </label>
    <input bind:value={config.proxy.tun.device_name} placeholder="utun7" />
  </div>
  {#if config.proxy.tun.enabled}
  <div class="per-app-section">
    <div class="per-app-header">
      <span class="per-app-label">{t('settings.perAppRouting')}</span>
      <select bind:value={config.proxy.tun.per_app_mode} class="per-app-mode">
        <option value="">{t('settings.perAppDisabled')}</option>
        <option value="allow">{t('settings.perAppAllow')}</option>
        <option value="deny">{t('settings.perAppDeny')}</option>
      </select>
    </div>
    {#if config.proxy.tun.per_app_mode}
      <div class="per-app-list">
        {#each config.proxy.tun.app_list as app, i}
          <div class="per-app-item">
            <span class="per-app-name">{app}</span>
            <button class="per-app-remove" onclick={() => onRemovePerApp(i)}>x</button>
          </div>
        {/each}
        <div class="per-app-add-row">
          <input
            bind:value={newAppName}
            placeholder="com.example.app or process name"
            class="per-app-input"
            onkeydown={(e) => e.key === 'Enter' && onAddPerApp(newAppName)}
          />
          <button class="per-app-add-btn" onclick={() => onAddPerApp(newAppName)}>{t('settings.add')}</button>
          <button class="per-app-pick-btn" onclick={onOpenPerAppPicker}>{t('settings.pick')}</button>
        </div>
      </div>
    {/if}
  </div>
  {/if}
  <div class="system-proxy-row">
    <label class="system-proxy-label">
      <input type="checkbox" bind:checked={config.proxy.allow_lan} onchange={onLoadLanInfo} />
      <span class="label-text">{t('settings.allowLan')}</span>
      <span class="hint">{t('settings.allowLanHint')}</span>
    </label>
  </div>
  {#if config.proxy.allow_lan && lanInfo?.addresses?.length > 0}
    <div class="lan-info">
      <div class="lan-info-title">{t('settings.lanAddresses')}</div>
      <div class="lan-info-list">
        {#each lanInfo.addresses as ip}
          <div class="lan-address">
            <span class="ip">{ip}</span>
            {#if config.proxy.socks5.enabled}
              <span class="port">SOCKS5: {ip}:{config.proxy.socks5.listen?.split(':')[1] || '1080'}</span>
            {/if}
            {#if config.proxy.http.enabled}
              <span class="port">HTTP: {ip}:{config.proxy.http.listen?.split(':')[1] || '8080'}</span>
            {/if}
          </div>
        {/each}
      </div>
      <div class="lan-info-hint">{t('settings.lanAddressesHint')}</div>
    </div>
  {/if}
  <div class="system-proxy-row">
    <label class="system-proxy-label">
      <input type="checkbox" bind:checked={config.proxy.system_proxy.enabled} />
      <span class="label-text">{t('settings.autoSystemProxy')}</span>
      <span class="hint">{t('settings.autoSystemProxyHint')}</span>
    </label>
  </div>
  <div class="system-proxy-row">
    <label class="system-proxy-label">
      <input
        type="checkbox"
        checked={autostartEnabled}
        onchange={onToggleAutostart}
        disabled={autostartLoading}
      />
      <span class="label-text">{t('settings.launchAtLogin')}</span>
      <span class="hint">{t('settings.launchAtLoginHint')}</span>
    </label>
  </div>
</section>

<style>
  section {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: 16px 20px;
    margin-bottom: 12px;
  }

  h3 { font-size: 14px; font-weight: 600; color: var(--text-primary); margin: 20px 0 10px; }

  .grid {
    display: grid;
    grid-template-columns: 120px 1fr;
    gap: 8px;
    align-items: center;
  }

  input[type="text"], input:not([type]) {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 6px 10px;
    color: var(--text-primary);
    font-size: 13px;
  }

  select {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 6px 10px;
    color: var(--text-primary);
    font-size: 13px;
  }

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

  .lan-info {
    margin-top: 12px;
    padding: 12px;
    background: rgba(56, 139, 253, 0.1);
    border: 1px solid #388bfd;
    border-radius: var(--radius-sm);
  }

  .lan-info-title {
    font-size: 12px;
    font-weight: 500;
    color: var(--accent);
    margin-bottom: 8px;
  }

  .lan-info-list {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .lan-address {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    font-size: 12px;
    font-family: 'Cascadia Code', 'Fira Code', monospace;
  }

  .lan-address .ip {
    color: var(--text-primary);
    font-weight: 500;
    min-width: 110px;
  }

  .lan-address .port {
    color: var(--text-secondary);
    background: var(--bg-tertiary);
    padding: 2px 6px;
    border-radius: 4px;
  }

  .lan-info-hint {
    font-size: 11px;
    color: var(--text-secondary);
    margin-top: 8px;
  }

  .per-app-section {
    margin-top: 12px;
    padding-top: 12px;
    border-top: 1px solid var(--border);
  }

  .per-app-header {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;
  }

  .per-app-label {
    font-size: 13px;
    color: var(--text-secondary);
    min-width: 100px;
  }

  .per-app-mode {
    flex: 1;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 6px 10px;
    color: var(--text-primary);
    font-size: 12px;
  }

  .per-app-list {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .per-app-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 6px 10px;
  }

  .per-app-name {
    font-size: 13px;
    color: var(--text-primary);
    font-family: 'Cascadia Code', 'Fira Code', monospace;
  }

  .per-app-remove {
    background: none;
    border: none;
    color: var(--accent-red);
    cursor: pointer;
    font-size: 14px;
    padding: 2px 6px;
  }

  .per-app-add-row {
    display: flex;
    gap: 4px;
    margin-top: 4px;
  }

  .per-app-input {
    flex: 1;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 6px 10px;
    color: var(--text-primary);
    font-size: 12px;
  }

  .per-app-add-btn, .per-app-pick-btn {
    background: var(--bg-tertiary);
    color: var(--accent);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 6px 12px;
    cursor: pointer;
    font-size: 12px;
  }

  .per-app-pick-btn { color: var(--accent-purple); }
</style>
