<script lang="ts">
  import { api } from '../lib/api'
  import { connectWS } from '../lib/ws'
  import { toast } from '../lib/toast'
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

  async function removeServer(addr) {
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
    if (!result.available) return { text: 'timeout', class: 'latency-error' }
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
  <h2>Active Server</h2>

  <div class="form">
    <label>
      <span>Server Address</span>
      <input bind:value={active.addr} placeholder="example.com:443" />
    </label>
    <label>
      <span>Name</span>
      <input bind:value={active.name} placeholder="My Server" />
    </label>
    <label>
      <span>Password</span>
      <input type="password" bind:value={active.password} />
    </label>

    <button onclick={save} disabled={saving}>
      {saving ? 'Saving...' : 'Save & Reconnect'}
    </button>
      </div>

  <div class="section-header">
    <h2 class="section">Saved Servers</h2>
    <div class="section-actions">
      <button class="btn-auto" onclick={autoSelect} disabled={autoSelecting || testing || servers.length === 0}>
        {autoSelecting ? 'Selecting...' : 'Auto Select'}
      </button>
      <button class="btn-test" onclick={runSpeedtest} disabled={testing || autoSelecting}>
        {#if testing}
          Testing ({testProgress.done}/{testProgress.total})...
        {:else}
          Test All
        {/if}
      </button>
      <button class="btn-import" onclick={() => (showImport = true)}>Import</button>
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
            <button class="btn-sm" onclick={() => switchTo(srv)}>Use</button>
            <button class="btn-sm btn-danger" onclick={() => removeServer(srv.addr)}>Remove</button>
          </div>
        </div>
      {/each}
    </div>
  {:else}
    <div class="empty-state">
      <div class="empty-icon">📡</div>
      <h3>No Servers Yet</h3>
      <p>Add a proxy server to get started</p>
      <div class="empty-actions">
        <button class="action-btn primary" onclick={() => showImport = true}>
          Import Config
        </button>
        <span class="or">or add manually below</span>
      </div>
    </div>
  {/if}

  <h3>Add Server</h3>
  <div class="add-form">
    <input bind:value={newServer.addr} placeholder="addr:port" />
    <input bind:value={newServer.name} placeholder="Name" />
    <input type="password" bind:value={newServer.password} placeholder="Password" />
    <button onclick={addServer}>Add</button>
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
        <h3 id="import-dialog-title">Import Servers</h3>
        <button class="modal-close" onclick={closeImport}>&times;</button>
      </div>
      <div class="modal-body">
        <p class="help-text">
          Paste configuration data below. Supported formats:
        </p>
        <ul class="help-list">
          <li>shuttle:// URI (one per line)</li>
          <li>JSON (single server or array)</li>
          <li>Base64 encoded JSON</li>
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
              Added {importResult.added} of {importResult.total} server(s)
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
        <button class="btn-cancel" onclick={closeImport}>Cancel</button>
        <button class="btn-primary" onclick={doImport} disabled={importing || !importData.trim()}>
          {importing ? 'Importing...' : 'Import'}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  .page { max-width: 560px; }
  h2 { font-size: 18px; margin-bottom: 20px; }
  h2.section { margin-top: 32px; }
  h3 { font-size: 14px; color: #8b949e; margin: 20px 0 10px; }

  .form { display: flex; flex-direction: column; gap: 14px; }

  label {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  label span {
    font-size: 12px;
    color: #8b949e;
  }

  input {
    background: #161b22;
    border: 1px solid #2d333b;
    border-radius: 6px;
    padding: 8px 12px;
    color: #e1e4e8;
    font-size: 14px;
  }

  input:focus {
    outline: none;
    border-color: #58a6ff;
  }

  button {
    background: #238636;
    color: #fff;
    border: none;
    border-radius: 6px;
    padding: 10px;
    cursor: pointer;
    font-size: 14px;
    margin-top: 8px;
  }

  button:hover { background: #2ea043; }
  button:disabled { opacity: 0.5; }

  .msg { font-size: 13px; color: #8b949e; margin-top: 4px; }
  .empty-state {
    text-align: center;
    padding: 40px 20px;
    background: #161b22;
    border: 1px dashed #2d333b;
    border-radius: 12px;
    margin: 20px 0;
  }

  .empty-icon {
    font-size: 48px;
    margin-bottom: 16px;
  }

  .empty-state h3 {
    font-size: 18px;
    color: #e1e4e8;
    margin: 0 0 8px;
  }

  .empty-state p {
    font-size: 14px;
    color: #8b949e;
    margin: 0 0 20px;
  }

  .empty-actions {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
  }

  .action-btn.primary {
    background: #238636;
    color: white;
    border: none;
    padding: 12px 24px;
    border-radius: 8px;
    font-size: 14px;
    font-weight: 500;
    cursor: pointer;
  }

  .action-btn.primary:hover {
    background: #2ea043;
  }

  .or {
    font-size: 12px;
    color: #8b949e;
  }

  .server-list { display: flex; flex-direction: column; gap: 8px; }

  .server-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    background: #161b22;
    border: 1px solid #2d333b;
    border-radius: 6px;
    padding: 10px 14px;
  }

  .server-name { font-size: 14px; color: #e1e4e8; }
  .server-addr { font-size: 12px; color: #484f58; margin-left: 8px; }
  .server-actions { display: flex; gap: 6px; }

  .btn-sm {
    padding: 4px 10px;
    font-size: 12px;
    margin-top: 0;
    background: #21262d;
  }
  .btn-sm:hover { background: #30363d; }
  .btn-danger { color: #f85149; }
  .btn-danger:hover { background: #3d1f1f; }

  .add-form {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px;
  }
  .add-form button {
    grid-column: span 2;
    margin-top: 0;
  }

  .section-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .section-actions {
    display: flex;
    gap: 8px;
  }

  .btn-auto {
    background: #238636;
    color: #fff;
    border: 1px solid #238636;
    border-radius: 6px;
    padding: 6px 14px;
    cursor: pointer;
    font-size: 13px;
    margin-top: 0;
  }
  .btn-auto:hover { background: #2ea043; }
  .btn-auto:disabled { opacity: 0.5; cursor: default; }

  .btn-test {
    background: #21262d;
    color: #3fb950;
    border: 1px solid #2d333b;
    border-radius: 6px;
    padding: 6px 14px;
    cursor: pointer;
    font-size: 13px;
    margin-top: 0;
  }
  .btn-test:hover { background: #30363d; }
  .btn-test:disabled { opacity: 0.7; cursor: default; }

  .latency {
    font-size: 12px;
    padding: 2px 6px;
    border-radius: 4px;
    margin-left: 8px;
  }

  .latency-good {
    background: rgba(63, 185, 80, 0.2);
    color: #3fb950;
  }

  .latency-ok {
    background: rgba(210, 153, 34, 0.2);
    color: #d29922;
  }

  .latency-slow {
    background: rgba(248, 81, 73, 0.2);
    color: #f85149;
  }

  .latency-error {
    background: rgba(248, 81, 73, 0.2);
    color: #f85149;
  }

  .btn-import {
    background: #21262d;
    color: #58a6ff;
    border: 1px solid #2d333b;
    border-radius: 6px;
    padding: 6px 14px;
    cursor: pointer;
    font-size: 13px;
    margin-top: 0;
  }
  .btn-import:hover { background: #30363d; }

  .modal-overlay {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background: rgba(0, 0, 0, 0.7);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
  }

  .modal {
    background: #161b22;
    border: 1px solid #2d333b;
    border-radius: 12px;
    width: 90%;
    max-width: 480px;
    max-height: 90vh;
    overflow: hidden;
  }

  .modal-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 16px;
    border-bottom: 1px solid #2d333b;
  }

  .modal-header h3 {
    margin: 0;
    font-size: 16px;
    color: #e1e4e8;
  }

  .modal-close {
    background: none;
    border: none;
    color: #8b949e;
    font-size: 24px;
    cursor: pointer;
    padding: 0;
    line-height: 1;
    margin-top: 0;
  }
  .modal-close:hover { color: #e1e4e8; }

  .modal-body {
    padding: 16px;
  }

  .help-text {
    font-size: 13px;
    color: #8b949e;
    margin: 0 0 8px;
  }

  .help-list {
    font-size: 12px;
    color: #6e7681;
    margin: 0 0 12px;
    padding-left: 20px;
  }

  .modal-body textarea {
    width: 100%;
    background: #0d1117;
    border: 1px solid #2d333b;
    border-radius: 6px;
    padding: 10px;
    color: #e1e4e8;
    font-size: 13px;
    font-family: 'Cascadia Code', 'Fira Code', monospace;
    resize: vertical;
    box-sizing: border-box;
  }

  .modal-body textarea:focus {
    outline: none;
    border-color: #58a6ff;
  }

  .import-error {
    color: #f85149;
    font-size: 13px;
    margin: 8px 0 0;
  }

  .import-success {
    color: #3fb950;
    font-size: 13px;
    margin: 8px 0 0;
  }

  .import-errors {
    color: #d29922;
    font-size: 12px;
    margin: 4px 0 0;
    padding-left: 20px;
  }

  .modal-footer {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    padding: 12px 16px;
    border-top: 1px solid #2d333b;
  }

  .btn-cancel {
    background: #21262d;
    color: #e1e4e8;
    margin-top: 0;
  }
  .btn-cancel:hover { background: #30363d; }

  .btn-primary {
    background: #238636;
    margin-top: 0;
  }
  .btn-primary:hover { background: #2ea043; }
  .btn-primary:disabled { opacity: 0.5; }
</style>
