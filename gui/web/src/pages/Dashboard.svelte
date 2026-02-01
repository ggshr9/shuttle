<script>
  import { api } from '../lib/api.js'
  import { connectWS } from '../lib/ws.js'
  import { onMount } from 'svelte'

  let status = $state(null)
  let connected = $state(false)
  let speed = $state({ upload: 0, download: 0 })
  let loading = $state(false)

  onMount(() => {
    refresh()
    const interval = setInterval(refresh, 3000)
    const ws = connectWS('/api/speed', (ev) => {
      speed = { upload: ev.upload || 0, download: ev.download || 0 }
    })
    return () => {
      clearInterval(interval)
      ws.close()
    }
  })

  async function refresh() {
    try {
      status = await api.status()
      connected = status.state === 'running'
    } catch {
      // API unavailable, keep last state
    }
  }

  async function toggle() {
    loading = true
    try {
      if (connected) {
        await api.disconnect()
      } else {
        await api.connect()
      }
      await refresh()
    } finally {
      loading = false
    }
  }

  function fmt(bytes) {
    if (bytes < 1024) return bytes + ' B/s'
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB/s'
    return (bytes / 1024 / 1024).toFixed(1) + ' MB/s'
  }
</script>

<div class="dashboard">
  <div class="hero">
    <button class="toggle" class:on={connected} onclick={toggle} disabled={loading}>
      <span class="icon">{connected ? '⬤' : '○'}</span>
    </button>
    <p class="state">{status?.state ?? 'loading...'}</p>
  </div>

  <div class="speed-cards">
    <div class="card">
      <span class="label">Upload</span>
      <span class="value">{fmt(speed.upload)}</span>
    </div>
    <div class="card">
      <span class="label">Download</span>
      <span class="value">{fmt(speed.download)}</span>
    </div>
  </div>

  {#if status}
    <div class="stats">
      <div class="stat">
        <span>Active Connections</span>
        <span>{status.active_conns}</span>
      </div>
      <div class="stat">
        <span>Total Connections</span>
        <span>{status.total_conns}</span>
      </div>
      <div class="stat">
        <span>Transport</span>
        <span>{status.transport || 'none'}</span>
      </div>
    </div>

    {#if status.transports?.length}
      <h3>Transports</h3>
      <div class="transports">
        {#each status.transports as t}
          <div class="transport" class:available={t.available}>
            <span>{t.type}</span>
            <span>{t.available ? `${t.latency_ms}ms` : 'unavailable'}</span>
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</div>

<style>
  .dashboard { text-align: center; }

  .hero { margin: 30px 0; }

  .toggle {
    width: 100px;
    height: 100px;
    border-radius: 50%;
    border: 3px solid #2d333b;
    background: #161b22;
    cursor: pointer;
    transition: all 0.3s;
    display: flex;
    align-items: center;
    justify-content: center;
    margin: 0 auto;
  }

  .toggle:hover { border-color: #58a6ff; }
  .toggle.on { border-color: #3fb950; background: #0d1117; }
  .toggle:disabled { opacity: 0.5; }

  .icon { font-size: 32px; color: #8b949e; }
  .toggle.on .icon { color: #3fb950; }

  .state {
    margin-top: 12px;
    font-size: 14px;
    color: #8b949e;
    text-transform: uppercase;
    letter-spacing: 1px;
  }

  .speed-cards {
    display: flex;
    gap: 16px;
    justify-content: center;
    margin: 24px 0;
  }

  .card {
    background: #161b22;
    border: 1px solid #2d333b;
    border-radius: 8px;
    padding: 16px 32px;
    min-width: 140px;
  }

  .card .label {
    display: block;
    font-size: 12px;
    color: #8b949e;
    margin-bottom: 4px;
  }

  .card .value {
    font-size: 20px;
    font-weight: 600;
    color: #e1e4e8;
  }

  .stats {
    background: #161b22;
    border: 1px solid #2d333b;
    border-radius: 8px;
    padding: 16px;
    margin: 16px 0;
  }

  .stat {
    display: flex;
    justify-content: space-between;
    padding: 8px 0;
    border-bottom: 1px solid #21262d;
  }

  .stat:last-child { border-bottom: none; }

  h3 {
    font-size: 14px;
    color: #8b949e;
    margin: 20px 0 8px;
    text-align: left;
  }

  .transports {
    display: flex;
    gap: 8px;
  }

  .transport {
    background: #161b22;
    border: 1px solid #2d333b;
    border-radius: 6px;
    padding: 10px 16px;
    display: flex;
    gap: 12px;
    font-size: 13px;
  }

  .transport.available { border-color: #3fb950; }
</style>
