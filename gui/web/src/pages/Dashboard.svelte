<script lang="ts">
  import { api, type TransportStats } from '../lib/api'
  import { connectWS } from '../lib/ws'
  import { onMount } from 'svelte'
  import { requestPermission, notifyConnected, notifyDisconnected } from '../lib/notify'
  import { initShortcuts, registerShortcut, destroyShortcuts, getShortcutDisplay, isMac } from '../lib/shortcuts'
  import SpeedChart from '../lib/SpeedChart.svelte'
  import ConnectionQualityChart from '../lib/ConnectionQualityChart.svelte'
  import MeshTopologyChart from '../lib/MeshTopologyChart.svelte'
  import TrafficChart from '../lib/TrafficChart.svelte'
  import SpeedTestHistory from '../lib/SpeedTestHistory.svelte'
  import { t } from '../lib/i18n/index'

  let status = $state(null)
  let connected = $state(false)
  let speed = $state({ upload: 0, download: 0 })
  let loading = $state(false)
  let prevConnected = $state(null)
  let notificationsEnabled = $state(false)
  let transportStats: TransportStats[] = $state([])

  // Real-time speed history for chart (last 5 minutes = 60 data points at 5s intervals)
  const MAX_CHART_POINTS = 60
  let uploadHistory = $state([])
  let downloadHistory = $state([])

  // Shortcut display text
  const toggleShortcut = getShortcutDisplay('k', { [isMac ? 'meta' : 'ctrl']: true })

  onMount(() => {
    refresh()
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
    try {
      if (connected) {
        transportStats = await api.getTransportStats() || []
      } else {
        transportStats = []
      }
    } catch {
      // Transport stats not available
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

    {#if transportStats.length > 0}
      <h3>{t('dashboard.transportBreakdown')}</h3>
      <div class="transport-breakdown">
        <table>
          <thead>
            <tr>
              <th class="col-transport">{t('dashboard.transport')}</th>
              <th class="col-num">{t('dashboard.activeStreams')}</th>
              <th class="col-num">{t('dashboard.totalStreams')}</th>
              <th class="col-num">{t('dashboard.sent')}</th>
              <th class="col-num">{t('dashboard.received')}</th>
            </tr>
          </thead>
          <tbody>
            {#each transportStats as ts}
              <tr>
                <td class="col-transport">{ts.transport}</td>
                <td class="col-num">{ts.active_streams}</td>
                <td class="col-num">{ts.total_streams}</td>
                <td class="col-num">{formatBytes(ts.bytes_sent)}</td>
                <td class="col-num">{formatBytes(ts.bytes_recv)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
        <div class="transport-bars">
          {#each transportStats as ts}
            {@const maxStreams = Math.max(...transportStats.map(s => s.total_streams), 1)}
            <div class="transport-bar-row">
              <span class="transport-bar-label">{ts.transport}</span>
              <div class="transport-bar-track">
                <div class="transport-bar-fill active" style="width: {(ts.active_streams / maxStreams) * 100}%"></div>
                <div class="transport-bar-fill total" style="width: {((ts.total_streams - ts.active_streams) / maxStreams) * 100}%"></div>
              </div>
            </div>
          {/each}
        </div>
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

  <SpeedTestHistory />

  <TrafficChart />
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

  .transport-breakdown {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 16px;
    margin: 8px 0;
  }

  .transport-breakdown table {
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;
    text-align: left;
  }

  .transport-breakdown th {
    color: var(--text-secondary);
    font-weight: 500;
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    padding: 6px 8px;
    border-bottom: 1px solid var(--border);
  }

  .transport-breakdown td {
    padding: 8px;
    color: var(--text-primary);
    border-bottom: 1px solid var(--bg-tertiary);
  }

  .transport-breakdown tr:last-child td {
    border-bottom: none;
  }

  .col-transport {
    text-align: left;
  }

  .col-num {
    text-align: right;
  }

  .transport-bars {
    margin-top: 12px;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .transport-bar-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .transport-bar-label {
    font-size: 11px;
    color: var(--text-secondary);
    min-width: 60px;
    text-align: right;
  }

  .transport-bar-track {
    flex: 1;
    height: 8px;
    background: var(--bg-tertiary);
    border-radius: 4px;
    display: flex;
    overflow: hidden;
  }

  .transport-bar-fill.active {
    background: var(--accent-green);
  }

  .transport-bar-fill.total {
    background: var(--accent);
    opacity: 0.4;
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
