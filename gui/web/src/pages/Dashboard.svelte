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
  let apiError = $state(false)

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
      apiError = false
    } catch {
      apiError = true
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
  {#if apiError}
    <div class="api-error-banner">
      <span class="api-error-icon">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><path d="M8 1a7 7 0 100 14A7 7 0 008 1zm-.75 3.75a.75.75 0 011.5 0v3.5a.75.75 0 01-1.5 0v-3.5zM8 11a1 1 0 110 2 1 1 0 010-2z"/></svg>
      </span>
      <span>{t('dashboard.connectionLost')}</span>
    </div>
  {/if}

  <!-- Connection Hero -->
  <div class="hero-card" class:connected>
    <div class="hero-top">
      <div class="status-info">
        <div class="status-row">
          <span class="status-dot" class:online={connected}></span>
          <span class="status-text">{status?.state ? t('dashboard.state.' + status.state) : t('dashboard.state.loading')}</span>
        </div>
        <p class="shortcut-hint">{toggleShortcut}</p>
      </div>
      <button class="toggle-btn" class:on={connected} onclick={toggle} disabled={loading} title="Toggle connection ({toggleShortcut})">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          {#if connected}
            <path d="M18.36 6.64a9 9 0 01.203 12.519M5.637 5.637a9 9 0 00-.203 12.519"/>
            <path d="M15.54 8.46a5 5 0 01.1 6.96M8.46 8.46a5 5 0 00-.1 6.96"/>
            <circle cx="12" cy="12" r="1" fill="currentColor"/>
          {:else}
            <line x1="1" y1="1" x2="23" y2="23" />
            <path d="M16.72 11.06A10.94 10.94 0 0119 12.55M5 12.55a10.94 10.94 0 015.17-2.39"/>
            <path d="M10.71 5.05A16 16 0 0122.56 9M1.42 9a15.91 15.91 0 014.7-2.88"/>
            <path d="M8.53 16.11a6 6 0 016.95 0"/>
            <line x1="12" y1="20" x2="12.01" y2="20"/>
          {/if}
        </svg>
        <span class="toggle-label">{connected ? t('dashboard.disconnect') || 'Disconnect' : t('dashboard.connect') || 'Connect'}</span>
      </button>
    </div>

    {#if connected}
      <div class="speed-row">
        <div class="speed-item">
          <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="var(--accent-green)" stroke-width="1.5"><path d="M7 11V3m0 0L3 7m4-4l4 4"/></svg>
          <span class="speed-label">{t('dashboard.upload')}</span>
          <span class="speed-value">{fmt(speed.upload)}</span>
        </div>
        <div class="speed-divider"></div>
        <div class="speed-item">
          <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="var(--accent)" stroke-width="1.5"><path d="M7 3v8m0 0l4-4m-4 4L3 7"/></svg>
          <span class="speed-label">{t('dashboard.download')}</span>
          <span class="speed-value">{fmt(speed.download)}</span>
        </div>
      </div>
    {/if}
  </div>

  <!-- Stats Grid -->
  {#if status}
    <div class="stats-grid">
      <div class="stat-card">
        <span class="stat-label">{t('dashboard.totalUpload')}</span>
        <span class="stat-value">{formatBytes(status.bytes_sent || 0)}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">{t('dashboard.totalDownload')}</span>
        <span class="stat-value">{formatBytes(status.bytes_received || 0)}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">{t('dashboard.activeConnections')}</span>
        <span class="stat-value">{status.active_conns}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">{t('dashboard.transport')}</span>
        <span class="stat-value transport-badge">{status.transport || 'none'}</span>
      </div>
    </div>
  {/if}

  <!-- Realtime Chart -->
  {#if uploadHistory.length > 1 || downloadHistory.length > 1}
    <div class="section-card">
      <div class="section-header">
        <h3>{t('dashboard.realtimeTraffic')}</h3>
        <div class="chart-legend">
          <span class="legend-item"><span class="legend-dot upload"></span> {t('dashboard.upload')}</span>
          <span class="legend-item"><span class="legend-dot download"></span> {t('dashboard.download')}</span>
        </div>
      </div>
      <SpeedChart
        uploadData={uploadHistory}
        downloadData={downloadHistory}
        maxPoints={MAX_CHART_POINTS}
        height={140}
      />
    </div>
  {/if}

  <!-- Transports -->
  {#if status?.transports?.length}
    <div class="section-card">
      <h3>{t('dashboard.transports')}</h3>
      <div class="transports-grid">
        {#each status.transports as tr}
          <div class="transport-item" class:available={tr.available}>
            <span class="transport-name">{tr.type}</span>
            <span class="transport-latency">{tr.available ? `${tr.latency_ms}ms` : t('dashboard.unavailable')}</span>
          </div>
        {/each}
      </div>
    </div>
  {/if}

  <!-- Transport Breakdown -->
  {#if transportStats.length > 0}
    <div class="section-card">
      <h3>{t('dashboard.transportBreakdown')}</h3>
      <div class="table-wrap">
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
      </div>
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

  <!-- Mesh VPN -->
  {#if status?.mesh?.enabled}
    <div class="section-card">
      <h3>{t('dashboard.meshVPN') || 'Mesh VPN'}</h3>
      {#if status.mesh.virtual_ip}
        <div class="mesh-cards">
          <div class="mesh-item">
            <span class="mesh-label">{t('dashboard.virtualIP')}</span>
            <span class="mesh-value">{status.mesh.virtual_ip}</span>
          </div>
          <div class="mesh-item">
            <span class="mesh-label">{t('dashboard.network')}</span>
            <span class="mesh-value">{status.mesh.cidr}</span>
          </div>
        </div>
      {/if}
      {#if status.mesh.peers?.length > 0}
        <h4 class="subsection-title">{t('dashboard.topology') || 'Network Topology'}</h4>
        <MeshTopologyChart
          peers={status.mesh.peers}
          selfIP={status.mesh.virtual_ip}
        />
        <h4 class="subsection-title">{t('dashboard.connectionQuality') || 'Connection Quality'}</h4>
      {/if}
      <ConnectionQualityChart peers={status.mesh.peers || []} height={150} />
    </div>
  {/if}

  <SpeedTestHistory />

  <TrafficChart />
</div>

<style>
  .dashboard {
    max-width: 860px;
  }

  /* ===== Hero Card ===== */
  .hero-card {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: 24px;
    margin-bottom: 20px;
    transition: border-color 0.3s;
  }

  .hero-card.connected {
    border-color: var(--accent-green);
    box-shadow: 0 0 0 1px var(--accent-green-subtle), 0 4px 24px var(--accent-green-subtle);
  }

  .hero-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .status-info {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .status-row {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .status-dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    background: var(--text-muted);
    transition: background 0.3s;
  }

  .status-dot.online {
    background: var(--accent-green);
    box-shadow: 0 0 8px var(--accent-green);
  }

  .status-text {
    font-size: 18px;
    font-weight: 600;
    text-transform: capitalize;
    letter-spacing: -0.01em;
  }

  .shortcut-hint {
    font-size: 12px;
    color: var(--text-muted);
    margin: 0;
    padding-left: 20px;
    font-family: 'Inter', monospace;
  }

  .toggle-btn {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 20px;
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
    border-radius: var(--radius-xl);
    color: var(--text-secondary);
    cursor: pointer;
    font-size: 14px;
    font-weight: 500;
    transition: all 0.2s;
  }

  .toggle-btn:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
    border-color: var(--border-light);
  }

  .toggle-btn.on {
    background: var(--accent-green-subtle);
    border-color: var(--accent-green);
    color: var(--accent-green);
  }

  .toggle-btn.on:hover {
    background: rgba(52, 211, 153, 0.2);
  }

  .toggle-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .toggle-label {
    white-space: nowrap;
  }

  .speed-row {
    display: flex;
    align-items: center;
    gap: 16px;
    margin-top: 20px;
    padding-top: 20px;
    border-top: 1px solid var(--border);
  }

  .speed-item {
    display: flex;
    align-items: center;
    gap: 8px;
    flex: 1;
  }

  .speed-label {
    font-size: 13px;
    color: var(--text-secondary);
  }

  .speed-value {
    font-size: 15px;
    font-weight: 600;
    color: var(--text-primary);
    font-variant-numeric: tabular-nums;
    margin-left: auto;
  }

  .speed-divider {
    width: 1px;
    height: 24px;
    background: var(--border);
  }

  /* ===== Stats Grid ===== */
  .stats-grid {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 12px;
    margin-bottom: 20px;
  }

  .stat-card {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .stat-label {
    font-size: 12px;
    color: var(--text-secondary);
    font-weight: 500;
  }

  .stat-value {
    font-size: 18px;
    font-weight: 600;
    color: var(--text-primary);
    font-variant-numeric: tabular-nums;
  }

  .transport-badge {
    font-family: 'Inter', monospace;
    font-size: 14px !important;
    color: var(--accent) !important;
  }

  /* ===== Section Cards ===== */
  .section-card {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: 20px;
    margin-bottom: 20px;
  }

  .section-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 16px;
  }

  h3 {
    font-size: 14px;
    font-weight: 600;
    color: var(--text-primary);
    margin: 0 0 16px;
    letter-spacing: -0.01em;
  }

  .section-header h3 {
    margin: 0;
  }

  .chart-legend {
    display: flex;
    gap: 16px;
  }

  .legend-item {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--text-secondary);
  }

  .legend-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
  }

  .legend-dot.upload { background: var(--accent-green); }
  .legend-dot.download { background: var(--accent); }

  /* ===== Transports ===== */
  .transports-grid {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }

  .transport-item {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    padding: 10px 16px;
    display: flex;
    gap: 12px;
    font-size: 13px;
    align-items: center;
  }

  .transport-item.available {
    border-color: var(--accent-green);
    background: var(--accent-green-subtle);
  }

  .transport-name {
    font-weight: 500;
  }

  .transport-latency {
    color: var(--text-secondary);
    font-variant-numeric: tabular-nums;
  }

  .transport-item.available .transport-latency {
    color: var(--accent-green);
  }

  /* ===== Table ===== */
  .table-wrap {
    overflow-x: auto;
  }

  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;
  }

  th {
    color: var(--text-secondary);
    font-weight: 500;
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    padding: 8px 10px;
    border-bottom: 1px solid var(--border);
    text-align: left;
  }

  td {
    padding: 10px;
    color: var(--text-primary);
    border-bottom: 1px solid var(--border);
  }

  tr:last-child td {
    border-bottom: none;
  }

  .col-transport { text-align: left; }
  .col-num { text-align: right; }
  th.col-num { text-align: right; }

  .transport-bars {
    margin-top: 16px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .transport-bar-row {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .transport-bar-label {
    font-size: 12px;
    color: var(--text-secondary);
    min-width: 60px;
    text-align: right;
    font-weight: 500;
  }

  .transport-bar-track {
    flex: 1;
    height: 6px;
    background: var(--bg-tertiary);
    border-radius: 3px;
    display: flex;
    overflow: hidden;
  }

  .transport-bar-fill.active {
    background: var(--accent-green);
    border-radius: 3px;
  }

  .transport-bar-fill.total {
    background: var(--accent);
    opacity: 0.35;
  }

  /* ===== Mesh ===== */
  .mesh-cards {
    display: flex;
    gap: 12px;
    margin-bottom: 16px;
  }

  .mesh-item {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    padding: 12px 16px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .mesh-label {
    font-size: 11px;
    color: var(--text-secondary);
    font-weight: 500;
  }

  .mesh-value {
    font-size: 14px;
    font-family: 'JetBrains Mono', 'Fira Code', monospace;
    color: var(--accent);
  }

  .subsection-title {
    font-size: 12px;
    color: var(--text-secondary);
    margin: 16px 0 10px;
    font-weight: 500;
  }

  /* ===== Error Banner ===== */
  .api-error-banner {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 12px 16px;
    background: var(--accent-yellow-subtle);
    border: 1px solid var(--accent-yellow);
    border-radius: var(--radius-md);
    margin-bottom: 20px;
    font-size: 13px;
    color: var(--accent-yellow);
  }

  .api-error-icon {
    display: flex;
    align-items: center;
  }

  /* ===== Responsive ===== */
  @media (max-width: 640px) {
    .stats-grid {
      grid-template-columns: repeat(2, 1fr);
    }
  }
</style>
