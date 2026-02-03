<script>
  import { api } from '../lib/api.js'
  import { t, getLocale, setLocale, getLocales } from '../lib/i18n/index.js'
  import { onMount } from 'svelte'

  let config = $state(null)
  let saving = $state(false)
  let restoring = $state(false)
  let msg = $state('')
  let selectedLocale = $state(getLocale())
  let availableLocales = getLocales()
  let fileInput = $state(null)

  // Update state
  let currentVersion = $state('')
  let updateInfo = $state(null)
  let checkingUpdate = $state(false)
  let showChangelog = $state(false)

  onMount(async () => {
    config = await api.getConfig()
    // Get current version and check for updates
    try {
      const v = await api.getVersion()
      currentVersion = v.version
      // Auto-check for updates on load
      checkForUpdate(false)
    } catch {
      currentVersion = 'unknown'
    }
  })

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
      msg = `Restored: ${res.servers} servers, ${res.subscriptions} subscriptions`
      // Reload config after restore
      config = await api.getConfig()
    } catch (err) {
      msg = 'Restore failed: ' + (err.message || err)
    } finally {
      restoring = false
      e.target.value = ''
    }
  }
</script>

{#if config}
<div class="page">
  <h2>Settings</h2>

  <section>
    <h3>Proxy Listeners</h3>
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
  </section>

  <section>
    <h3>DNS</h3>
    <label class="row">
      <span>Domestic DNS</span>
      <input bind:value={config.routing.dns.domestic} placeholder="223.5.5.5" />
    </label>
    <label class="row">
      <span>Remote DNS</span>
      <input bind:value={config.routing.dns.remote.server} placeholder="https://1.1.1.1/dns-query" />
    </label>
    <label class="row">
      <span>Remote Via</span>
      <select bind:value={config.routing.dns.remote.via}>
        <option value="proxy">Proxy</option>
        <option value="direct">Direct</option>
      </select>
    </label>
    <div class="checkbox-row">
      <label>
        <input type="checkbox" bind:checked={config.routing.dns.cache} />
        Enable DNS Cache
      </label>
      <label>
        <input type="checkbox" bind:checked={config.routing.dns.prefetch} />
        Enable DNS Prefetch
      </label>
    </div>
  </section>

  <section>
    <h3>Log</h3>
    <label class="row">
      <span>Level</span>
      <select bind:value={config.log.level}>
        <option value="debug">Debug</option>
        <option value="info">Info</option>
        <option value="warn">Warn</option>
        <option value="error">Error</option>
      </select>
    </label>
  </section>

  <button class="save" onclick={save} disabled={saving}>
    {saving ? 'Saving...' : 'Save & Reload'}
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
    <h3>Backup & Restore</h3>
    <p class="section-hint">Full backup includes servers, subscriptions, routing rules, and all settings.</p>
    <div class="backup-buttons">
      <a href={api.backupUrl()} download="shuttle-backup.json" class="backup-btn">
        <span class="btn-icon">📦</span>
        Create Backup
      </a>
      <button class="backup-btn restore" onclick={triggerRestore} disabled={restoring}>
        <span class="btn-icon">📥</span>
        {restoring ? 'Restoring...' : 'Restore from Backup'}
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
  </section>

  <section class="update-section">
    <h3>Updates</h3>
    <div class="version-info">
      <span class="version-label">Current Version:</span>
      <span class="version-value">{currentVersion}</span>
    </div>

    {#if updateInfo?.available}
      <div class="update-available">
        <div class="update-badge">
          <span class="badge-icon">🎉</span>
          New version available: {updateInfo.latest_version}
        </div>
        <div class="update-actions">
          <button class="changelog-btn" onclick={() => (showChangelog = !showChangelog)}>
            {showChangelog ? 'Hide' : 'Show'} Changelog
          </button>
          <a
            href={updateInfo.release_url}
            target="_blank"
            rel="noopener noreferrer"
            class="download-btn"
          >
            View Release
          </a>
          {#if updateInfo.download_url}
            <a
              href={updateInfo.download_url}
              class="download-btn primary"
            >
              Download ({formatSize(updateInfo.asset_size)})
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
      <p class="up-to-date">✓ You're running the latest version</p>
    {/if}

    <button
      class="check-btn"
      onclick={() => checkForUpdate(true)}
      disabled={checkingUpdate}
    >
      {checkingUpdate ? 'Checking...' : 'Check for Updates'}
    </button>
  </section>
</div>
{:else}
<p>Loading...</p>
{/if}

<style>
  .page { max-width: 500px; }
  h2 { font-size: 18px; margin-bottom: 20px; }
  h3 { font-size: 14px; color: #8b949e; margin: 20px 0 10px; }

  section {
    background: #161b22;
    border: 1px solid #2d333b;
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

  .row span { font-size: 13px; color: #8b949e; min-width: 100px; }

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
    color: #8b949e;
  }

  input[type="text"], input:not([type]) {
    background: #0d1117;
    border: 1px solid #2d333b;
    border-radius: 6px;
    padding: 6px 10px;
    color: #e1e4e8;
    font-size: 13px;
  }

  select {
    background: #0d1117;
    border: 1px solid #2d333b;
    border-radius: 6px;
    padding: 6px 10px;
    color: #e1e4e8;
    font-size: 13px;
  }

  .save {
    background: #238636;
    color: #fff;
    border: none;
    border-radius: 6px;
    padding: 10px 20px;
    cursor: pointer;
    font-size: 14px;
    margin-top: 16px;
  }

  .save:disabled { opacity: 0.5; }
  .msg { font-size: 13px; color: #8b949e; margin-top: 8px; }

  .export-section {
    margin-top: 24px;
  }

  .export-buttons {
    display: flex;
    gap: 12px;
  }

  .export-btn {
    background: #21262d;
    color: #58a6ff;
    border: 1px solid #2d333b;
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
    background: #21262d;
    color: #e1e4e8;
    border: 1px solid #2d333b;
    border-radius: 6px;
    padding: 10px 16px;
    text-decoration: none;
    font-size: 13px;
    cursor: pointer;
    transition: all 0.2s;
  }

  .backup-btn:hover {
    background: #30363d;
    border-color: #3fb950;
  }

  .backup-btn.restore {
    background: #161b22;
    border-color: #58a6ff;
    color: #58a6ff;
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
    color: #8b949e;
    font-size: 13px;
  }

  .version-value {
    color: #e1e4e8;
    font-size: 13px;
    font-family: 'Cascadia Code', 'Fira Code', monospace;
  }

  .update-available {
    background: rgba(63, 185, 80, 0.08);
    border: 1px solid #3fb950;
    border-radius: 8px;
    padding: 12px;
    margin-bottom: 12px;
  }

  .update-badge {
    display: flex;
    align-items: center;
    gap: 8px;
    color: #3fb950;
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
    border: 1px solid #3fb950;
    color: #3fb950;
    border-radius: 6px;
    padding: 6px 12px;
    cursor: pointer;
    font-size: 12px;
  }

  .download-btn {
    background: #21262d;
    border: 1px solid #2d333b;
    color: #58a6ff;
    border-radius: 6px;
    padding: 6px 12px;
    text-decoration: none;
    font-size: 12px;
    cursor: pointer;
  }

  .download-btn.primary {
    background: #238636;
    border-color: #238636;
    color: #fff;
  }

  .download-btn:hover {
    background: #30363d;
  }

  .download-btn.primary:hover {
    background: #2ea043;
  }

  .changelog {
    margin-top: 12px;
    background: #0d1117;
    border: 1px solid #21262d;
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
    color: #3fb950;
    font-size: 13px;
    margin-bottom: 12px;
  }

  .check-btn {
    background: #21262d;
    border: 1px solid #2d333b;
    color: #e1e4e8;
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
