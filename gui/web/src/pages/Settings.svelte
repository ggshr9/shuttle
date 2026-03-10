<script lang="ts">
  import { api } from '../lib/api'
  import { t, getLocale, setLocale, getLocales } from '../lib/i18n/index'
  import { getTheme, setTheme, type Theme } from '../lib/theme'
  import { onMount } from 'svelte'

  let config = $state(null)
  let saving = $state(false)
  let restoring = $state(false)
  let msg = $state('')
  let selectedLocale = $state(getLocale())
  let availableLocales = getLocales()
  let selectedTheme = $state(getTheme())
  let fileInput = $state(null)

  // GeoData state
  let geoStatus = $state(null)
  let updatingGeo = $state(false)

  // Update state
  let currentVersion = $state('')
  let updateInfo = $state(null)
  let checkingUpdate = $state(false)
  let showChangelog = $state(false)

  // Autostart state
  let autostartEnabled = $state(false)
  let autostartLoading = $state(false)

  // LAN sharing state
  let lanInfo = $state(null)

  // Per-app routing state
  let perAppProcesses = $state([])
  let showPerAppPicker = $state(false)
  let newAppName = $state('')

  onMount(async () => {
    config = await api.getConfig()
    // Ensure system_proxy exists in config
    if (!config.proxy.system_proxy) {
      config.proxy.system_proxy = { enabled: false }
    }
    // Ensure TUN per-app fields exist
    if (!config.proxy.tun) {
      config.proxy.tun = { enabled: false, device_name: '', per_app_mode: '', app_list: [] }
    }
    if (!config.proxy.tun.app_list) {
      config.proxy.tun.app_list = []
    }
    if (!config.proxy.tun.per_app_mode) {
      config.proxy.tun.per_app_mode = ''
    }
    // Ensure qos exists in config
    if (!config.qos) {
      config.qos = { enabled: false, rules: [] }
    }
    if (!config.qos.rules) {
      config.qos.rules = []
    }
    // Ensure geodata exists in config
    if (!config.routing) {
      config.routing = { rules: [], default: 'proxy', geodata: { enabled: true, auto_update: true } }
    }
    if (!config.routing.geodata) {
      config.routing.geodata = { enabled: true, auto_update: true }
    }
    // Get current version and check for updates
    try {
      const v = await api.getVersion()
      currentVersion = v.version
      // Auto-check for updates on load
      checkForUpdate(false)
    } catch {
      currentVersion = 'unknown'
    }
    // Load autostart status
    try {
      const as = await api.getAutostart()
      autostartEnabled = as.enabled
    } catch {
      // Autostart may not be supported on this platform
    }
    // Load LAN info
    loadLanInfo()
    // Load geodata status
    loadGeoStatus()
  })

  async function loadLanInfo() {
    try {
      lanInfo = await api.getLanInfo()
    } catch {
      lanInfo = null
    }
  }

  async function checkForUpdate(force = true) {
    checkingUpdate = true
    try {
      updateInfo = await api.checkUpdate(force)
    } catch (err) {
      console.error('Update check failed:', err)
    } finally {
      checkingUpdate = false
    }
  }

  function formatSize(bytes) {
    if (!bytes) return ''
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
    return (bytes / 1024 / 1024).toFixed(1) + ' MB'
  }

  function changeLocale(e) {
    const locale = e.target.value
    setLocale(locale)
    selectedLocale = locale
  }

  function changeTheme(e) {
    const theme = e.target.value as Theme
    setTheme(theme)
    selectedTheme = theme
  }

  async function openPerAppPicker() {
    try {
      perAppProcesses = await api.getProcesses()
      showPerAppPicker = true
    } catch (e) {
      msg = 'Failed to load processes: ' + e.message
    }
  }

  function addPerApp(name: string) {
    if (!name.trim()) return
    const list = config.proxy.tun.app_list || []
    if (!list.includes(name.trim())) {
      config.proxy.tun.app_list = [...list, name.trim()]
    }
    newAppName = ''
  }

  function removePerApp(index: number) {
    config.proxy.tun.app_list = config.proxy.tun.app_list.filter((_, i) => i !== index)
  }

  async function save() {
    saving = true
    msg = ''
    try {
      const res = await api.putConfig(config)
      msg = res.error || 'Saved & Reloaded'
    } finally {
      saving = false
    }
  }

  function triggerRestore() {
    fileInput?.click()
  }

  async function handleRestore(e) {
    const file = e.target.files?.[0]
    if (!file) return

    restoring = true
    msg = ''
    try {
      const text = await file.text()
      const backup = JSON.parse(text)
      const res = await api.restore(backup)
      msg = t('settings.restored', { servers: res.servers, subscriptions: res.subscriptions })
      // Reload config after restore
      config = await api.getConfig()
    } catch (err) {
      msg = t('settings.restoreFailed') + ': ' + (err.message || err)
    } finally {
      restoring = false
      e.target.value = ''
    }
  }

  async function loadGeoStatus() {
    try {
      geoStatus = await api.getGeoDataStatus()
    } catch { geoStatus = null }
  }

  async function updateGeoData() {
    updatingGeo = true
    try {
      geoStatus = await api.updateGeoData()
    } catch (err) {
      msg = 'GeoData update failed: ' + (err.message || err)
    } finally {
      updatingGeo = false
    }
  }

  async function toggleAutostart() {
    autostartLoading = true
    try {
      const newState = !autostartEnabled
      await api.setAutostart(newState)
      autostartEnabled = newState
    } catch (err) {
      msg = 'Autostart toggle failed: ' + (err.message || err)
    } finally {
      autostartLoading = false
    }
  }
</script>

{#if config}
<div class="page">
  <h2>{t('settings.title')}</h2>

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
        <span class="per-app-label">Per-App Routing</span>
        <select bind:value={config.proxy.tun.per_app_mode} class="per-app-mode">
          <option value="">Disabled</option>
          <option value="allow">Allow List (only these apps use proxy)</option>
          <option value="deny">Deny List (these apps bypass proxy)</option>
        </select>
      </div>
      {#if config.proxy.tun.per_app_mode}
        <div class="per-app-list">
          {#each config.proxy.tun.app_list as app, i}
            <div class="per-app-item">
              <span class="per-app-name">{app}</span>
              <button class="per-app-remove" onclick={() => removePerApp(i)}>x</button>
            </div>
          {/each}
          <div class="per-app-add-row">
            <input
              bind:value={newAppName}
              placeholder="com.example.app or process name"
              class="per-app-input"
              onkeydown={(e) => e.key === 'Enter' && addPerApp(newAppName)}
            />
            <button class="per-app-add-btn" onclick={() => addPerApp(newAppName)}>Add</button>
            <button class="per-app-pick-btn" onclick={openPerAppPicker}>Pick</button>
          </div>
        </div>
      {/if}
    </div>
    {/if}
    <div class="system-proxy-row">
      <label class="system-proxy-label">
        <input type="checkbox" bind:checked={config.proxy.allow_lan} onchange={loadLanInfo} />
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
          onchange={toggleAutostart}
          disabled={autostartLoading}
        />
        <span class="label-text">{t('settings.launchAtLogin')}</span>
        <span class="hint">{t('settings.launchAtLoginHint')}</span>
      </label>
    </div>
  </section>

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

  <section>
    <h3>{t('settings.log')}</h3>
    <label class="row">
      <span>{t('settings.logLevel')}</span>
      <select bind:value={config.log.level}>
        <option value="debug">Debug</option>
        <option value="info">Info</option>
        <option value="warn">Warn</option>
        <option value="error">Error</option>
      </select>
    </label>
  </section>

  <section>
    <h3>{t('settings.qos')}</h3>
    <div class="system-proxy-row" style="margin-top: 0; padding-top: 0; border-top: none;">
      <label class="system-proxy-label">
        <input type="checkbox" bind:checked={config.qos.enabled} />
        <span class="label-text">{t('settings.qosEnabled')}</span>
        <span class="hint">{t('settings.qosEnabledHint')}</span>
      </label>
    </div>
    {#if config.qos.enabled}
      <div class="qos-rules">
        <div class="qos-rules-header">
          <span class="qos-rules-title">{t('settings.qosRules')}</span>
        </div>
        {#if config.qos.rules?.length > 0}
          {#each config.qos.rules as rule, i}
            <div class="qos-rule">
              <select bind:value={rule.priority} class="qos-priority-select">
                <option value="critical">{t('settings.qosPriorities.critical')}</option>
                <option value="high">{t('settings.qosPriorities.high')}</option>
                <option value="normal">{t('settings.qosPriorities.normal')}</option>
                <option value="bulk">{t('settings.qosPriorities.bulk')}</option>
                <option value="low">{t('settings.qosPriorities.low')}</option>
              </select>
              <input
                type="text"
                placeholder={t('settings.qosPorts')}
                value={rule.ports?.join(', ') || ''}
                onchange={(e) => {
                  const ports = e.target.value.split(',').map(p => parseInt(p.trim())).filter(p => !isNaN(p))
                  rule.ports = ports.length > 0 ? ports : undefined
                }}
                class="qos-input"
              />
              <button class="qos-remove" onclick={() => config.qos.rules = config.qos.rules.filter((_, j) => j !== i)}>×</button>
            </div>
          {/each}
        {:else}
          <p class="qos-no-rules">{t('settings.qosNoRules')}</p>
        {/if}
        <button class="qos-add" onclick={() => config.qos.rules = [...(config.qos.rules || []), { priority: 'normal', ports: [] }]}>
          + {t('settings.qosAddRule')}
        </button>
      </div>
    {/if}
  </section>

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
        <button class="geo-update-btn" onclick={updateGeoData} disabled={updatingGeo}>
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

  <button class="save" onclick={save} disabled={saving}>
    {saving ? t('settings.saving') : t('settings.saveReload')}
  </button>
  {#if msg}<p class="msg">{msg}</p>{/if}

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

  <section>
    <h3>{t('settings.language')}</h3>
    <label class="row">
      <span>{t('settings.language')}</span>
      <select value={selectedLocale} onchange={changeLocale}>
        {#each availableLocales as locale}
          <option value={locale.code}>{locale.name}</option>
        {/each}
      </select>
    </label>
    <label class="row">
      <span>Theme</span>
      <select value={selectedTheme} onchange={changeTheme}>
        <option value="dark">Dark</option>
        <option value="light">Light</option>
      </select>
    </label>
  </section>

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
      onclick={() => checkForUpdate(true)}
      disabled={checkingUpdate}
    >
      {checkingUpdate ? t('settings.checking') : t('settings.checkUpdates')}
    </button>
  </section>
</div>
{:else}
<p>Loading...</p>
{/if}

{#if showPerAppPicker}
<div class="overlay" onclick={() => (showPerAppPicker = false)} role="dialog" aria-modal="true" aria-labelledby="perapp-picker-title" onkeydown={(e) => e.key === 'Escape' && (showPerAppPicker = false)}>
  <div class="picker-modal" onclick={(e) => e.stopPropagation()}>
    <h3 id="perapp-picker-title">Select Process</h3>
    <p class="picker-hint">Click a process to add it to the per-app list</p>
    {#if perAppProcesses.length}
      <div class="picker-list">
        {#each perAppProcesses as proc}
          <button class="picker-item" onclick={() => { addPerApp(proc.name); }}>
            <span class="picker-name">{proc.name}</span>
            <span class="picker-conns">{proc.conns} conn{proc.conns !== 1 ? 's' : ''}</span>
          </button>
        {/each}
      </div>
    {:else}
      <p class="picker-empty">No processes with active connections found</p>
    {/if}
    <button class="picker-close" onclick={() => (showPerAppPicker = false)}>Done</button>
  </div>
</div>
{/if}

<style>
  .page { max-width: 500px; }
  h2 { font-size: 18px; margin-bottom: 20px; }
  h3 { font-size: 14px; color: var(--text-secondary); margin: 20px 0 10px; }

  section {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 12px 16px;
    margin-bottom: 12px;
  }

  .grid {
    display: grid;
    grid-template-columns: 120px 1fr;
    gap: 8px;
    align-items: center;
  }

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
    border-radius: 6px;
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

  .save {
    background: var(--btn-bg);
    color: #fff;
    border: none;
    border-radius: 6px;
    padding: 10px 20px;
    cursor: pointer;
    font-size: 14px;
    margin-top: 16px;
  }

  .save:disabled { opacity: 0.5; }
  .msg { font-size: 13px; color: var(--text-secondary); margin-top: 8px; }

  .export-section {
    margin-top: 24px;
  }

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

  .backup-section {
    margin-top: 24px;
  }

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

  .update-section {
    margin-top: 24px;
  }

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

  /* QoS styles */
  .qos-rules {
    margin-top: 12px;
  }

  .qos-rules-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 8px;
  }

  .qos-rules-title {
    font-size: 12px;
    color: var(--text-secondary);
  }

  .qos-rule {
    display: flex;
    gap: 8px;
    align-items: center;
    margin-bottom: 8px;
  }

  .qos-priority-select {
    min-width: 140px;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 6px 10px;
    color: var(--text-primary);
    font-size: 12px;
  }

  .qos-input {
    flex: 1;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 6px 10px;
    color: var(--text-primary);
    font-size: 12px;
  }

  .qos-remove {
    background: transparent;
    border: 1px solid var(--accent-red);
    color: var(--accent-red);
    border-radius: 4px;
    width: 24px;
    height: 24px;
    cursor: pointer;
    font-size: 14px;
    line-height: 1;
  }

  .qos-remove:hover {
    background: rgba(248, 81, 73, 0.1);
  }

  .qos-no-rules {
    font-size: 12px;
    color: #6e7681;
    font-style: italic;
    margin: 8px 0;
  }

  .qos-add {
    background: transparent;
    border: 1px dashed var(--border);
    color: var(--text-secondary);
    border-radius: 6px;
    padding: 8px 12px;
    cursor: pointer;
    font-size: 12px;
    width: 100%;
    margin-top: 8px;
  }

  .qos-add:hover {
    border-color: var(--accent);
    color: var(--accent);
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

  /* Per-app routing */
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
    border-radius: 6px;
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
    border-radius: 6px;
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
    border-radius: 6px;
    padding: 6px 10px;
    color: var(--text-primary);
    font-size: 12px;
  }

  .per-app-add-btn, .per-app-pick-btn {
    background: var(--bg-tertiary);
    color: var(--accent);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 6px 12px;
    cursor: pointer;
    font-size: 12px;
  }

  .per-app-pick-btn { color: var(--accent-purple); }

  /* Process picker modal */
  .overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
  }

  .picker-modal {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 20px;
    width: 400px;
    max-height: 500px;
    display: flex;
    flex-direction: column;
  }

  .picker-modal h3 { font-size: 16px; margin: 0 0 4px; color: var(--text-primary); }
  .picker-hint { font-size: 12px; color: var(--text-muted); margin: 0 0 12px; }

  .picker-list {
    overflow-y: auto;
    max-height: 350px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .picker-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px 12px;
    cursor: pointer;
    color: var(--text-primary);
    font-size: 13px;
    width: 100%;
    text-align: left;
  }

  .picker-item:hover { border-color: var(--accent); }
  .picker-name { font-weight: 500; }
  .picker-conns { font-size: 11px; color: var(--text-muted); }
  .picker-empty { font-size: 13px; color: var(--text-muted); }

  .picker-close {
    margin-top: 12px;
    background: var(--bg-tertiary);
    color: var(--text-primary);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px;
    cursor: pointer;
    font-size: 13px;
  }
</style>
