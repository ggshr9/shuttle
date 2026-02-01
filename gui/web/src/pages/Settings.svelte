<script>
  import { api } from '../lib/api.js'
  import { onMount } from 'svelte'

  let config = $state(null)
  let saving = $state(false)
  let msg = $state('')

  onMount(async () => {
    config = await api.getConfig()
  })

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

  .row span { font-size: 13px; color: #8b949e; min-width: 80px; }

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
</style>
