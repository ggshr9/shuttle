<script>
  import { api } from '../lib/api.js'
  import { onMount } from 'svelte'

  let server = $state({ addr: '', name: '', password: '', sni: '' })
  let saving = $state(false)
  let msg = $state('')

  onMount(async () => {
    server = await api.getServers()
  })

  async function save() {
    saving = true
    msg = ''
    try {
      const res = await api.putServers(server)
      msg = res.error || 'Saved'
    } finally {
      saving = false
    }
  }
</script>

<div class="page">
  <h2>Server Configuration</h2>

  <div class="form">
    <label>
      <span>Server Address</span>
      <input bind:value={server.addr} placeholder="example.com:443" />
    </label>
    <label>
      <span>Name</span>
      <input bind:value={server.name} placeholder="My Server" />
    </label>
    <label>
      <span>Password</span>
      <input type="password" bind:value={server.password} />
    </label>
    <label>
      <span>SNI</span>
      <input bind:value={server.sni} placeholder="example.com" />
    </label>

    <button onclick={save} disabled={saving}>
      {saving ? 'Saving...' : 'Save & Reconnect'}
    </button>
    {#if msg}<p class="msg">{msg}</p>{/if}
  </div>
</div>

<style>
  .page { max-width: 500px; }
  h2 { font-size: 18px; margin-bottom: 20px; }

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
</style>
