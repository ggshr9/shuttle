<script>
  import { api } from '../lib/api.js'
  import { onMount } from 'svelte'

  let active = $state({ addr: '', name: '', password: '', sni: '' })
  let servers = $state([])
  let saving = $state(false)
  let msg = $state('')
  let newServer = $state({ addr: '', name: '', password: '', sni: '' })

  onMount(async () => {
    try {
      const data = await api.getServers()
      active = data.active || { addr: '', name: '', password: '', sni: '' }
      servers = data.servers || []
    } catch (e) {
      msg = 'Failed to load: ' + e.message
    }
  })

  async function save() {
    saving = true
    msg = ''
    try {
      await api.putServers(active)
      msg = 'Saved & reconnecting...'
    } catch (e) {
      msg = e.message
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
    try {
      await api.addServer(newServer)
      servers = [...servers, { ...newServer }]
      newServer = { addr: '', name: '', password: '', sni: '' }
    } catch (e) {
      msg = e.message
    }
  }

  async function removeServer(addr) {
    try {
      await api.deleteServer(addr)
      servers = servers.filter(s => s.addr !== addr)
    } catch (e) {
      msg = e.message
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
    <label>
      <span>SNI</span>
      <input bind:value={active.sni} placeholder="example.com" />
    </label>

    <button onclick={save} disabled={saving}>
      {saving ? 'Saving...' : 'Save & Reconnect'}
    </button>
    {#if msg}<p class="msg">{msg}</p>{/if}
  </div>

  <h2 class="section">Saved Servers</h2>

  {#if servers.length}
    <div class="server-list">
      {#each servers as srv}
        <div class="server-item">
          <div class="server-info">
            <span class="server-name">{srv.name || srv.addr}</span>
            <span class="server-addr">{srv.addr}</span>
          </div>
          <div class="server-actions">
            <button class="btn-sm" onclick={() => switchTo(srv)}>Use</button>
            <button class="btn-sm btn-danger" onclick={() => removeServer(srv.addr)}>Remove</button>
          </div>
        </div>
      {/each}
    </div>
  {:else}
    <p class="empty">No saved servers</p>
  {/if}

  <h3>Add Server</h3>
  <div class="add-form">
    <input bind:value={newServer.addr} placeholder="addr:port" />
    <input bind:value={newServer.name} placeholder="Name" />
    <input type="password" bind:value={newServer.password} placeholder="Password" />
    <input bind:value={newServer.sni} placeholder="SNI" />
    <button onclick={addServer}>Add</button>
  </div>
</div>

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
  .empty { font-size: 13px; color: #484f58; }

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
</style>
