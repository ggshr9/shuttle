<script lang="ts">
  import { api } from '../lib/api'
  import { connectWS } from '../lib/ws'
  import { onMount } from 'svelte'
  import { requestPermission, notifyConnected, notifyDisconnected } from '../lib/notify'
  import { initShortcuts, registerShortcut, destroyShortcuts, getShortcutDisplay, isMac } from '../lib/shortcuts'
  import SpeedChart from '../lib/SpeedChart.svelte'
  import ConnectionQualityChart from '../lib/ConnectionQualityChart.svelte'
  import MeshTopologyChart from '../lib/MeshTopologyChart.svelte'
  import { t } from '../lib/i18n/index'

  let status = $state(null)
  let connected = $state(false)
  let speed = $state({ upload: 0, download: 0 })
  let loading = $state(false)
  let history = $state([])
  let prevConnected = $state(null)
  let notificationsEnabled = $state(false)

  // Real-time speed history for chart (last 5 minutes = 60 data points at 5s intervals)
  const MAX_CHART_POINTS = 60
  let uploadHistory = $state([])
  let downloadHistory = $state([])

  // Shortcut display text
  const toggleShortcut = getShortcutDisplay('k', { [isMac ? 'meta' : 'ctrl']: true })

  onMount(() => {
    refresh()
    loadHistory()
    // Request notification permission on mount
    requestPermission().then(granted => {
      notificationsEnabled = granted
    })

    // Initialize keyboard shortcuts
    initShortcuts()
    // Cmd/Ctrl+K to toggle connection
    const unregisterToggle = registerShortcut('k', () => {
      if (!loading) toggle()
    }, { [isMac ? 'meta' : 'ctrl']: true })

    const interval = setInterval(refresh, 3000)
    const ws = connectWS('/api/speed', (ev) => {
      speed = { upload: ev.upload || 0, download: ev.download || 0 }
      // Add to speed history for chart
      uploadHistory = [...uploadHistory.slice(-(MAX_CHART_POINTS - 1)), ev.upload || 0]
      downloadHistory = [...downloadHistory.slice(-(MAX_CHART_POINTS - 1)), ev.download || 0]
    })
    return () => {
      clearInterval(interval)
      ws.close()
      unregisterToggle()
    }
  })

  // Watch for connection state changes and notify
  $effect(() => {
    if (prevConnected !== null && connected !== prevConnected && notificationsEnabled) {
      if (connected) {
        notifyConnected(status?.server_name || status?.server_addr)
      } else {
        notifyDisconnected()
      }
    }
    prevConnected = connected
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

  function formatBytes(bytes) {
    if (bytes < 1024) return bytes + ' B'
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
    if (bytes < 1024 * 1024 * 1024) return (bytes / 1024 / 1024).toFixed(2) + ' MB'
    return (bytes / 1024 / 1024 / 1024).toFixed(2) + ' GB'
  }

  async function loadHistory() {
    try {
      const data = await api.getStatsHistory(7)
      history = data.history || []
    } catch {
      // Stats not available
    }
  }

  function getMaxTraffic(h) {
    return Math.max(...h.map(d => d.bytes_sent + d.bytes_received), 1)
  }

  function getDayLabel(dateStr) {
    const date = new Date(dateStr)
    const days = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
    return days[date.getDay()]
  }
</script>

<div class="dashboard">
  <div class="hero">
    <button class="toggle" class:on={connected} onclick={toggle} disabled={loading} title="Toggle connection ({toggleShortcut})">
      <span class="icon">{connected ? '⬤' : '○'}</span>
    </button>
    <p class="state">{status?.state ? t('dashboard.state.' + status.state) : t('dashboard.state.loading')}</p>
    <p class="shortcut-hint">Press {toggleShortcut} to toggle</p>
  </div>

  <div class="speed-cards">
    <div class="card">
      <span class="label">{t('dashboard.upload')}</span>
      <span class="value">{fmt(speed.upload)}</span>
    </div>
    <div class="card">
      <span class="label">{t('dashboard.download')}</span>
      <span class="value">{fmt(speed.download)}</span>
    </div>
  </div>

  {#if uploadHistory.length > 1 || downloadHistory.length > 1}
    <div class="realtime-chart">
      <h3>{t('dashboard.realtimeTraffic')}</h3>
      <SpeedChart
        uploadData={uploadHistory}
        downloadData={downloadHistory}
        maxPoints={MAX_CHART_POINTS}
        height={120}
      />
      <div class="chart-legend inline">
        <span class="legend-item"><span class="legend-color upload"></span> {t('dashboard.upload')}</span>
        <span class="legend-item"><span class="legend-color download"></span> {t('dashboard.download')}</span>
      </div>
    </div>
  {/if}

  {#if status}
    <div class="traffic-cards">
      <div class="card traffic">
        <span class="label">{t('dashboard.totalUpload')}</span>
        <span class="value">{formatBytes(status.bytes_sent || 0)}</span>
      </div>
      <div class="card traffic">
        <span class="label">{t('dashboard.totalDownload')}</span>
        <span class="value">{formatBytes(status.bytes_received || 0)}</span>
      </div>
    </div>
  {/if}

  {#if status}
    <div class="stats">
      <div class="stat">
        <span>{t('dashboard.activeConnections')}</span>
        <span>{status.active_conns}</span>
      </div>
      <div class="stat">
        <span>{t('dashboard.totalConnections')}</span>
        <span>{status.total_conns}</span>
      </div>
      <div class="stat">
        <span>{t('dashboard.transport')}</span>
        <span>{status.transport || 'none'}</span>
      </div>
    </div>

    {#if status.transports?.length}
      <h3>{t('dashboard.transports')}</h3>
      <div class="transports">
        {#each status.transports as tr}
          <div class="transport" class:available={tr.available}>
            <span>{tr.type}</span>
            <span>{tr.available ? `${tr.latency_ms}ms` : t('dashboard.unavailable')}</span>
          </div>
        {/each}
      </div>
    {/if}

    {#if status.mesh?.enabled}
      <h3>{t('dashboard.meshVPN') || 'Mesh VPN'}</h3>
      <div class="mesh-info">
        {#if status.mesh.virtual_ip}
          <div class="mesh-status">
            <span class="mesh-label">Virtual IP</span>
            <span class="mesh-value">{status.mesh.virtual_ip}</span>
          </div>
          <div class="mesh-status">
            <span class="mesh-label">Network</span>
            <span class="mesh-value">{status.mesh.cidr}</span>
          </div>
        {/if}
      </div>
      {#if status.mesh.peers?.length > 0}
        <h4 class="subsection-title">{t('dashboard.topology') || 'Network Topology'}</h4>
        <MeshTopologyChart
          peers={status.mesh.peers}
          selfIP={status.mesh.virtual_ip}
        />
        <h4 class="subsection-title">{t('dashboard.connectionQuality') || 'Connection Quality'}</h4>
      {/if}
      <ConnectionQualityChart peers={status.mesh.peers || []} height={150} />
    {/if}
  {/if}

  {#if history.length > 0}
    <h3>{t('dashboard.trafficHistory')}</h3>
    <div class="history-chart">
      {#each history as day}
        {@const maxTraffic = getMaxTraffic(history)}
        {@const totalBytes = day.bytes_sent + day.bytes_received}
        {@const heightPercent = Math.max((totalBytes / maxTraffic) * 100, 2)}
        <div class="chart-bar-container">
          <div class="chart-bar" style="height: {heightPercent}%">
            <div class="bar-upload" style="height: {day.bytes_sent / (totalBytes || 1) * 100}%"></div>
            <div class="bar-download" style="height: {day.bytes_received / (totalBytes || 1) * 100}%"></div>
          </div>
          <span class="chart-label">{getDayLabel(day.date)}</span>
          <span class="chart-value">{formatBytes(totalBytes)}</span>
        </div>
      {/each}
    </div>
    <div class="chart-legend">
      <span class="legend-item"><span class="legend-color upload"></span> {t('dashboard.upload')}</span>
      <span class="legend-item"><span class="legend-color download"></span> {t('dashboard.download')}</span>
    </div>
  {/if}
</div>

<style>
  .dashboard { text-align: center; }

  .hero { margin: 30px 0; }

  .toggle {
    width: 100px;
    height: 100px;
    border-radius: 50%;
    border: 3px solid var(--border);
    background: var(--bg-secondary);
    cursor: pointer;
    transition: all 0.3s;
    display: flex;
    align-items: center;
    justify-content: center;
    margin: 0 auto;
  }

  .toggle:hover { border-color: var(--accent); }
  .toggle.on { border-color: var(--accent-green); background: var(--bg-surface); }
  .toggle:disabled { opacity: 0.5; }

  .icon { font-size: 32px; color: var(--text-secondary); }
  .toggle.on .icon { color: var(--accent-green); }

  .state {
    margin-top: 12px;
    font-size: 14px;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 1px;
  }

  .shortcut-hint {
    margin-top: 8px;
    font-size: 11px;
    color: var(--text-muted);
  }

  .speed-cards {
    display: flex;
    gap: 16px;
    justify-content: center;
    margin: 24px 0;
  }

  .card {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 16px 32px;
    min-width: 140px;
  }

  .card .label {
    display: block;
    font-size: 12px;
    color: var(--text-secondary);
    margin-bottom: 4px;
  }

  .card .value {
    font-size: 20px;
    font-weight: 600;
    color: var(--text-primary);
  }

  .traffic-cards {
    display: flex;
    gap: 16px;
    justify-content: center;
    margin: 16px 0;
  }

  .card.traffic {
    background: var(--bg-surface);
    border-color: var(--btn-bg);
  }

  .card.traffic .value {
    color: var(--accent-green);
  }

  .stats {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 16px;
    margin: 16px 0;
  }

  .stat {
    display: flex;
    justify-content: space-between;
    padding: 8px 0;
    border-bottom: 1px solid var(--bg-tertiary);
  }

  .stat:last-child { border-bottom: none; }

  h3 {
    font-size: 14px;
    color: var(--text-secondary);
    margin: 20px 0 8px;
    text-align: left;
  }

  .transports {
    display: flex;
    gap: 8px;
  }

  .transport {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 10px 16px;
    display: flex;
    gap: 12px;
    font-size: 13px;
  }

  .transport.available { border-color: var(--accent-green); }

  .history-chart {
    display: flex;
    gap: 8px;
    justify-content: center;
    align-items: flex-end;
    height: 120px;
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 16px;
    margin: 8px 0;
  }

  .chart-bar-container {
    display: flex;
    flex-direction: column;
    align-items: center;
    flex: 1;
    max-width: 60px;
  }

  .chart-bar {
    width: 100%;
    max-width: 40px;
    display: flex;
    flex-direction: column;
    border-radius: 4px 4px 0 0;
    overflow: hidden;
    min-height: 2px;
  }

  .bar-upload {
    background: var(--accent);
  }

  .bar-download {
    background: var(--accent-green);
  }

  .chart-label {
    font-size: 10px;
    color: var(--text-secondary);
    margin-top: 4px;
  }

  .chart-value {
    font-size: 9px;
    color: #6e7681;
  }

  .chart-legend {
    display: flex;
    justify-content: center;
    gap: 16px;
    margin-top: 8px;
  }

  .legend-item {
    display: flex;
    align-items: center;
    gap: 4px;
    font-size: 11px;
    color: var(--text-secondary);
  }

  .legend-color {
    width: 12px;
    height: 12px;
    border-radius: 2px;
  }

  .legend-color.upload {
    background: var(--accent);
  }

  .legend-color.download {
    background: var(--accent-green);
  }

  .realtime-chart {
    margin: 24px 0;
  }

  .realtime-chart h3 {
    text-align: left;
  }

  .chart-legend.inline {
    margin-top: 8px;
  }

  .mesh-info {
    display: flex;
    gap: 16px;
    margin-bottom: 12px;
  }

  .mesh-status {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 10px 16px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .mesh-label {
    font-size: 11px;
    color: var(--text-secondary);
  }

  .mesh-value {
    font-size: 14px;
    font-family: monospace;
    color: var(--accent);
  }

  .subsection-title {
    font-size: 12px;
    color: #6e7681;
    margin: 12px 0 8px;
    text-align: left;
    font-weight: normal;
  }
</style>
