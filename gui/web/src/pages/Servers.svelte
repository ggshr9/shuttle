<script lang="ts">
  import { api } from '../lib/api'
  import { connectWS } from '../lib/ws'
  import { toast } from '../lib/toast'
  import { t } from '../lib/i18n/index'
  import { onMount } from 'svelte'

  let active = $state({ addr: '', name: '', password: '' })
  let servers = $state([])
  let saving = $state(false)
  let newServer = $state({ addr: '', name: '', password: '' })

  // Import dialog state
  let showImport = $state(false)
  let importData = $state('')
  let importing = $state(false)
  let importResult = $state(null)

  // Speedtest state
  let testing = $state(false)
  let testProgress = $state({ done: 0, total: 0 })
  let testResults = $state({}) // addr -> result

  // Delete confirmation state
  let confirmDelete = $state(null) // server addr to delete

  // Auto-select state
  let autoSelecting = $state(false)

  onMount(async () => {
    try {
      const data = await api.getServers()
      active = data.active || { addr: '', name: '', password: '' }
      servers = data.servers || []
    } catch (e) {
      toast.error('Failed to load servers: ' + (e as Error).message)
    }
  })

  async function save() {
    saving = true
    try {
      await api.putServers(active)
      toast.success('Saved & reconnecting...')
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      saving = false
    }
  }

  async function switchTo(srv) {
    active = { ...srv }
    await save()
  }

  async function addServer() {
    if (!newServer.addr) return
    if (servers.some(s => s.addr === newServer.addr)) {
      toast.error('Server with this address already exists')
      return
    }
    try {
      await api.addServer(newServer)
      servers = [...servers, { ...newServer }]
      newServer = { addr: '', name: '', password: '' }
      toast.success('Server added')
    } catch (e) {
      toast.error((e as Error).message)
    }
  }

  function removeServer(addr) {
    confirmDelete = addr
  }

  async function confirmRemove() {
    const addr = confirmDelete
    confirmDelete = null
    try {
      await api.deleteServer(addr)
      const idx = servers.findIndex(s => s.addr === addr)
      if (idx !== -1) servers = [...servers.slice(0, idx), ...servers.slice(idx + 1)]
      toast.success('Server removed')
    } catch (e) {
      toast.error((e as Error).message)
    }
  }

  async function doImport() {
    if (!importData.trim()) return
    importing = true
    importResult = null
    try {
      const result = await api.importConfig(importData)
      importResult = result
      // Refresh server list
      const data = await api.getServers()
      servers = data.servers || []
      if (result.added > 0) {
        toast.success(`Imported ${result.added} server(s)`)
      }
    } catch (e) {
      importResult = { error: e.message }
    } finally {
      importing = false
    }
  }

  function closeImport() {
    showImport = false
    importData = ''
    importResult = null
  }

  function runSpeedtest() {
    testing = true
    testProgress = { done: 0, total: 0 }
    testResults = {}

    const ws = connectWS('/api/speedtest/stream', (data) => {
      if (data.total) {
        testProgress.total = data.total
      }
      if (data.result) {
        testResults[data.result.server_addr] = data.result
        testProgress.done++
        testResults = { ...testResults } // trigger reactivity
        testProgress = { ...testProgress }
      }
      if (data.done) {
        testing = false
        ws.close()
      }
      if (data.error) {
        toast.error(data.error)
        testing = false
        ws.close()
      }
    })
  }

  function getLatencyDisplay(addr) {
    const result = testResults[addr]
    if (!result) return null
    if (!result.available) return { text: t('common.timeout'), class: 'latency-error' }
    const ms = result.latency
    if (ms < 100) return { text: `${ms}ms`, class: 'latency-good' }
    if (ms < 300) return { text: `${ms}ms`, class: 'latency-ok' }
    return { text: `${ms}ms`, class: 'latency-slow' }
  }

  async function autoSelect() {
    if (servers.length === 0) {
      toast.warning('No servers to select from')
      return
    }
    autoSelecting = true
    try {
      const result = await api.autoSelectServer()
      active = result.server
      toast.success(`Selected ${result.server.name || result.server.addr} (${result.latency}ms)`)
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      autoSelecting = false
    }
  }
</script>

<div class="page">
  <div class="page-header">
    <h2>{t('servers.activeServer')}</h2>
  </div>

  <div class="active-card">
    <div class="form">
      <label>
        <span>{t('servers.serverAddress')}</span>
        <input bind:value={active.addr} placeholder="example.com:443" />
      </label>
      <label>
        <span>{t('servers.name')}</span>
        <input bind:value={active.name} placeholder="My Server" />
      </label>
      <label>
        <span>{t('servers.password')}</span>
        <input type="password" bind:value={active.password} />
      </label>
      <button class="btn-primary" onclick={save} disabled={saving}>
        {saving ? t('servers.saving') : t('servers.saveReconnect')}
      </button>
    </div>
  </div>

  <div class="section-header">
    <h2>{t('servers.savedServers')}</h2>
    <div class="section-actions">
      <button class="btn-action accent" onclick={autoSelect} disabled={autoSelecting || testing || servers.length === 0}>
        {autoSelecting ? t('servers.selecting') : t('servers.autoSelect')}
      </button>
      <button class="btn-action green" onclick={runSpeedtest} disabled={testing || autoSelecting}>
        {#if testing}
          {t('servers.testing', { done: testProgress.done, total: testProgress.total })}
        {:else}
          {t('servers.testAll')}
        {/if}
      </button>
      <button class="btn-action" onclick={() => (showImport = true)}>{t('servers.import')}</button>
    </div>
  </div>

  {#if servers.length}
    <div class="server-list">
      {#each servers as srv}
        {@const latency = getLatencyDisplay(srv.addr)}
        <div class="server-item">
          <div class="server-info">
            <span class="server-name">{srv.name || srv.addr}</span>
            <span class="server-addr">{srv.addr}</span>
            {#if latency}
              <span class="latency {latency.class}">{latency.text}</span>
            {/if}
          </div>
          <div class="server-actions">
            <button class="btn-sm" onclick={() => switchTo(srv)}>{t('servers.use')}</button>
            <button class="btn-sm danger" onclick={() => removeServer(srv.addr)}>{t('servers.remove')}</button>
          </div>
        </div>
      {/each}
    </div>
  {:else}
    <div class="empty-state">
      <svg width="48" height="48" viewBox="0 0 48 48" fill="none" stroke="var(--text-muted)" stroke-width="1.5">
        <rect x="8" y="10" width="32" height="12" rx="3"/>
        <rect x="8" y="26" width="32" height="12" rx="3"/>
        <circle cx="14" cy="16" r="2" fill="var(--text-muted)"/>
        <circle cx="14" cy="32" r="2" fill="var(--text-muted)"/>
      </svg>
      <h3>{t('emptyState.noServers')}</h3>
      <p>{t('emptyState.addToStart')}</p>
      <div class="empty-actions">
        <button class="btn-primary" onclick={() => showImport = true}>
          {t('emptyState.importConfig')}
        </button>
        <span class="or">{t('emptyState.orAddManually')}</span>
      </div>
    </div>
  {/if}

  <div class="section-card">
    <h3>{t('servers.addServer')}</h3>
    <div class="add-form">
      <input bind:value={newServer.addr} placeholder="addr:port" />
      <input bind:value={newServer.name} placeholder={t('servers.name')} />
      <input type="password" bind:value={newServer.password} placeholder={t('servers.password')} />
      <button class="btn-primary" onclick={addServer}>{t('servers.add')}</button>
    </div>
  </div>
</div>

{#if showImport}
  <div
    class="modal-overlay"
    role="dialog"
    aria-modal="true"
    aria-labelledby="import-dialog-title"
    onclick={closeImport}
    onkeydown={(e) => e.key === 'Escape' && closeImport()}
  >
    <div class="modal" onclick={(e) => e.stopPropagation()}>
      <div class="modal-header">
        <h3 id="import-dialog-title">{t('import.title')}</h3>
        <button class="modal-close" onclick={closeImport}>&times;</button>
      </div>
      <div class="modal-body">
        <p class="help-text">
          {t('import.description')}
        </p>
        <ul class="help-list">
          <li>{t('import.formats.uri')}</li>
          <li>{t('import.formats.json')}</li>
          <li>{t('import.formats.base64')}</li>
        </ul>
        <textarea
          bind:value={importData}
          placeholder="shuttle://password@example.com:443?name=My Server"
          rows="6"
        ></textarea>
        {#if importResult}
          {#if importResult.error}
            <p class="import-error">{importResult.error}</p>
          {:else}
            <p class="import-success">
              {t('import.added', { added: importResult.added, total: importResult.total })}
            </p>
            {#if importResult.errors?.length}
              <ul class="import-errors">
                {#each importResult.errors as err}
                  <li>{err}</li>
                {/each}
              </ul>
            {/if}
          {/if}
        {/if}
      </div>
      <div class="modal-footer">
        <button class="btn-cancel" onclick={closeImport}>{t('import.cancel')}</button>
        <button class="btn-primary" onclick={doImport} disabled={importing || !importData.trim()}>
          {importing ? t('import.importing') : t('import.import')}
        </button>
      </div>
    </div>
  </div>
{/if}

{#if confirmDelete}
  <div
    class="modal-overlay"
    role="dialog"
    aria-modal="true"
    aria-labelledby="delete-confirm-title"
    onclick={() => (confirmDelete = null)}
    onkeydown={(e) => e.key === 'Escape' && (confirmDelete = null)}
  >
    <div class="modal" onclick={(e) => e.stopPropagation()}>
      <div class="modal-header">
        <h3 id="delete-confirm-title">{t('servers.confirmDelete')}</h3>
        <button class="modal-close" onclick={() => (confirmDelete = null)}>&times;</button>
      </div>
      <div class="modal-body">
        <p class="help-text">{t('servers.confirmDeleteMsg')}</p>
        <p class="delete-addr">{confirmDelete}</p>
      </div>
      <div class="modal-footer">
        <button class="btn-cancel" onclick={() => (confirmDelete = null)}>{t('common.cancel')}</button>
        <button class="btn-danger-confirm" onclick={confirmRemove}>{t('servers.remove')}</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .page { max-width: 680px; }

  .page-header { margin-bottom: 20px; }

  h2 {
    font-size: 18px;
    font-weight: 600;
    margin: 0;
    letter-spacing: -0.01em;
  }

  h3 {
    font-size: 14px;
    font-weight: 600;
    color: var(--text-primary);
    margin: 0 0 14px;
  }

  .active-card {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: 20px;
    margin-bottom: 28px;
  }

  .form { display: flex; flex-direction: column; gap: 14px; }

  label {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  label span {
    font-size: 12px;
    color: var(--text-secondary);
    font-weight: 500;
  }

  input {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 9px 12px;
    color: var(--text-primary);
    font-size: 14px;
    font-family: inherit;
    transition: border-color 0.15s;
  }

  input:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-subtle);
  }

  .btn-primary {
    background: var(--btn-bg);
    color: #fff;
    border: none;
    border-radius: var(--radius-sm);
    padding: 10px 16px;
    cursor: pointer;
    font-size: 14px;
    font-weight: 500;
    font-family: inherit;
    transition: background 0.15s;
  }

  .btn-primary:hover { background: var(--btn-bg-hover); }
  .btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }

  .section-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 14px;
    margin-top: 8px;
  }

  .section-actions {
    display: flex;
    gap: 8px;
  }

  .btn-action {
    background: var(--bg-tertiary);
    color: var(--text-secondary);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 7px 14px;
    cursor: pointer;
    font-size: 13px;
    font-weight: 500;
    font-family: inherit;
    transition: all 0.15s;
  }

  .btn-action:hover { background: var(--bg-hover); color: var(--text-primary); }
  .btn-action:disabled { opacity: 0.5; cursor: default; }
  .btn-action.accent { color: var(--accent); }
  .btn-action.green { color: var(--accent-green); }

  .empty-state {
    text-align: center;
    padding: 48px 24px;
    background: var(--bg-secondary);
    border: 1px dashed var(--border);
    border-radius: var(--radius-lg);
    margin: 20px 0;
  }

  .empty-state h3 {
    font-size: 16px;
    color: var(--text-primary);
    margin: 16px 0 8px;
  }

  .empty-state p {
    font-size: 14px;
    color: var(--text-secondary);
    margin: 0 0 24px;
  }

  .empty-actions {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
  }

  .or {
    font-size: 12px;
    color: var(--text-muted);
  }

  .server-list { display: flex; flex-direction: column; gap: 6px; }

  .server-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    padding: 12px 16px;
    transition: border-color 0.15s;
  }

  .server-item:hover {
    border-color: var(--border-light);
  }

  .server-info {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .server-name { font-size: 14px; font-weight: 500; color: var(--text-primary); }
  .server-addr { font-size: 12px; color: var(--text-muted); font-family: 'JetBrains Mono', monospace; }
  .server-actions { display: flex; gap: 6px; }

  .btn-sm {
    padding: 5px 12px;
    font-size: 12px;
    font-weight: 500;
    font-family: inherit;
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    color: var(--text-secondary);
    cursor: pointer;
    transition: all 0.15s;
  }
  .btn-sm:hover { background: var(--bg-hover); color: var(--text-primary); }
  .btn-sm.danger { color: var(--accent-red); }
  .btn-sm.danger:hover { background: var(--accent-red-subtle); }

  .latency {
    font-size: 12px;
    padding: 2px 8px;
    border-radius: 10px;
    font-weight: 500;
    font-variant-numeric: tabular-nums;
  }

  .latency-good {
    background: var(--accent-green-subtle);
    color: var(--accent-green);
  }

  .latency-ok {
    background: var(--accent-yellow-subtle);
    color: var(--accent-yellow);
  }

  .latency-slow {
    background: var(--accent-red-subtle);
    color: var(--accent-red);
  }

  .latency-error {
    background: var(--accent-red-subtle);
    color: var(--accent-red);
  }

  .section-card {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: 20px;
    margin-top: 20px;
  }

  .add-form {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 10px;
  }
  .add-form .btn-primary {
    grid-column: span 2;
  }

  /* ===== Modals ===== */
  .modal-overlay {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background: var(--overlay-bg);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
    backdrop-filter: blur(4px);
  }

  .modal {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    width: 90%;
    max-width: 480px;
    max-height: 90vh;
    overflow: hidden;
    box-shadow: var(--shadow-lg);
  }

  .modal-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 18px 20px;
    border-bottom: 1px solid var(--border);
  }

  .modal-header h3 {
    margin: 0;
    font-size: 16px;
    color: var(--text-primary);
  }

  .modal-close {
    background: none;
    border: none;
    color: var(--text-muted);
    font-size: 22px;
    cursor: pointer;
    padding: 0;
    line-height: 1;
    transition: color 0.15s;
  }
  .modal-close:hover { color: var(--text-primary); }

  .modal-body {
    padding: 20px;
  }

  .help-text {
    font-size: 13px;
    color: var(--text-secondary);
    margin: 0 0 8px;
  }

  .help-list {
    font-size: 12px;
    color: var(--text-muted);
    margin: 0 0 14px;
    padding-left: 20px;
  }

  .modal-body textarea {
    width: 100%;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 10px 12px;
    color: var(--text-primary);
    font-size: 13px;
    font-family: 'JetBrains Mono', 'Fira Code', monospace;
    resize: vertical;
    box-sizing: border-box;
  }

  .modal-body textarea:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-subtle);
  }

  .import-error { color: var(--accent-red); font-size: 13px; margin: 10px 0 0; }
  .import-success { color: var(--accent-green); font-size: 13px; margin: 10px 0 0; }
  .import-errors { color: var(--accent-yellow); font-size: 12px; margin: 4px 0 0; padding-left: 20px; }

  .modal-footer {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    padding: 14px 20px;
    border-top: 1px solid var(--border);
  }

  .btn-cancel {
    background: var(--bg-tertiary);
    color: var(--text-primary);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 8px 16px;
    cursor: pointer;
    font-size: 13px;
    font-family: inherit;
    font-weight: 500;
  }
  .btn-cancel:hover { background: var(--bg-hover); }

  .btn-danger-confirm {
    background: var(--accent-red);
    color: #fff;
    border: none;
    border-radius: var(--radius-sm);
    padding: 8px 16px;
    cursor: pointer;
    font-size: 13px;
    font-family: inherit;
    font-weight: 500;
  }
  .btn-danger-confirm:hover { opacity: 0.85; }

  .delete-addr {
    font-family: 'JetBrains Mono', 'Fira Code', monospace;
    font-size: 13px;
    color: var(--text-primary);
    background: var(--bg-surface);
    padding: 10px 14px;
    border-radius: var(--radius-sm);
    margin: 10px 0 0;
  }
</style>
