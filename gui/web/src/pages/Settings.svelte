<script lang="ts">
  import { api } from '../lib/api'
  import { t } from '../lib/i18n/index'
  import { onMount } from 'svelte'

  import SettingsProxy from '../lib/settings/SettingsProxy.svelte'
  import SettingsMesh from '../lib/settings/SettingsMesh.svelte'
  import SettingsDns from '../lib/settings/SettingsDns.svelte'
  import SettingsLog from '../lib/settings/SettingsLog.svelte'
  import SettingsQos from '../lib/settings/SettingsQos.svelte'
  import SettingsGeodata from '../lib/settings/SettingsGeodata.svelte'
  import SettingsAppearance from '../lib/settings/SettingsAppearance.svelte'
  import SettingsUpdate from '../lib/settings/SettingsUpdate.svelte'
  import SettingsExport from '../lib/settings/SettingsExport.svelte'
  import SettingsBackup from '../lib/settings/SettingsBackup.svelte'
  import SettingsDiagnostics from '../lib/settings/SettingsDiagnostics.svelte'
  import SettingsPerAppPicker from '../lib/settings/SettingsPerAppPicker.svelte'

  interface Props {
    onSwitchToSimple?: () => void
  }

  let { onSwitchToSimple }: Props = $props()

  let config = $state(null)
  let saving = $state(false)
  let msg = $state('')
  let msgType = $state('') // 'success' or 'error'

  // GeoData state
  let geoStatus = $state(null)
  let updatingGeo = $state(false)

  // Update state
  let currentVersion = $state('')
  let updateInfo = $state(null)
  let checkingUpdate = $state(false)

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
    // Ensure mesh exists in config
    if (!config.mesh) {
      config.mesh = { enabled: false, p2p_enabled: false }
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
    msgType = ''
    try {
      const res = await api.putConfig(config)
      if (res.error) {
        msg = res.error
        msgType = 'error'
      } else {
        msg = 'Saved & Reloaded'
        msgType = 'success'
      }
    } catch (err) {
      msg = err.message || 'Save failed'
      msgType = 'error'
    } finally {
      saving = false
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

  function handleMessage(message: string, type: string = '') {
    msg = message
    msgType = type
  }
</script>

{#if config}
<div class="page">
  <h2>{t('settings.title')}</h2>

  <SettingsProxy
    bind:config
    {lanInfo}
    {autostartEnabled}
    {autostartLoading}
    bind:newAppName
    onLoadLanInfo={loadLanInfo}
    onToggleAutostart={toggleAutostart}
    onOpenPerAppPicker={openPerAppPicker}
    onAddPerApp={addPerApp}
    onRemovePerApp={removePerApp}
  />

  <SettingsMesh bind:config />

  <SettingsDns bind:config />

  <SettingsLog bind:config />

  <SettingsQos bind:config />

  <SettingsGeodata
    bind:config
    {geoStatus}
    {updatingGeo}
    onUpdateGeoData={updateGeoData}
  />

  <button class="btn-save" onclick={save} disabled={saving}>
    {saving ? t('settings.saving') : t('settings.saveReload')}
  </button>
  {#if msg}<p class="msg" class:msg-success={msgType === 'success'} class:msg-error={msgType === 'error'}>{msg}</p>{/if}

  <SettingsExport />

  <SettingsBackup onMessage={handleMessage} />

  <SettingsAppearance />

  <SettingsUpdate
    {currentVersion}
    {updateInfo}
    {checkingUpdate}
    onCheckForUpdate={checkForUpdate}
  />

  <SettingsDiagnostics onMessage={handleMessage} />

  {#if onSwitchToSimple}
    <div class="mode-section">
      <button class="btn-simple-mode" onclick={onSwitchToSimple}>
        {t('settings.switchToSimple')}
      </button>
    </div>
  {/if}
</div>
{:else}
<p class="loading-text">{t('common.loading')}</p>
{/if}

<SettingsPerAppPicker
  bind:show={showPerAppPicker}
  processes={perAppProcesses}
  onAddPerApp={addPerApp}
/>

<style>
  .page { max-width: 600px; }

  h2 {
    font-size: 18px;
    font-weight: 600;
    margin: 0 0 20px;
    letter-spacing: -0.01em;
  }

  .btn-save {
    background: var(--btn-bg);
    color: #fff;
    border: none;
    border-radius: var(--radius-sm);
    padding: 10px 22px;
    cursor: pointer;
    font-size: 14px;
    font-weight: 500;
    font-family: inherit;
    margin-top: 16px;
    transition: background 0.15s;
  }

  .btn-save:hover { background: var(--btn-bg-hover); }
  .btn-save:disabled { opacity: 0.5; }

  .msg { font-size: 13px; color: var(--text-secondary); margin-top: 8px; }
  .msg-success { color: var(--accent-green); }
  .msg-error { color: var(--accent-red); }

  .loading-text { color: var(--text-secondary); font-size: 14px; }

  .mode-section {
    margin-top: 32px;
    padding-top: 24px;
    border-top: 1px solid var(--border);
  }

  .btn-simple-mode {
    background: none;
    border: 1px solid var(--border);
    color: var(--text-secondary);
    border-radius: var(--radius-sm);
    padding: 8px 16px;
    cursor: pointer;
    font-size: 13px;
    font-family: inherit;
    transition: border-color 0.15s, color 0.15s;
  }

  .btn-simple-mode:hover {
    border-color: var(--border-light);
    color: var(--text-primary);
  }
</style>
