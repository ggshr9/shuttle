<script lang="ts">
  import { api } from '../lib/api'
  import { connectWS, type WSConnection } from '../lib/ws'
  import { onMount } from 'svelte'
  import { t } from '../lib/i18n/index'

  interface Props {
    onSwitchMode?: () => void
  }

  let { onSwitchMode }: Props = $props()

  let connected = $state(false)
  let loading = $state(false)
  let loadingStatus = $state('')
  let serverName = $state('')
  let uploadSpeed = $state(0)
  let downloadSpeed = $state(0)
  let bytesSent = $state(0)
  let bytesReceived = $state(0)
  let apiError = $state(false)

  onMount(() => {
    refresh()
    const interval = setInterval(refresh, 3000)
    const ws: WSConnection = connectWS<{ upload: number; download: number }>('/api/speed', (data) => {
      uploadSpeed = data.upload || 0
      downloadSpeed = data.download || 0
    })
    return () => {
      clearInterval(interval)
      ws.close()
    }
  })

  async function refresh() {
    try {
      const s = await api.status()
      connected = s.connected || (s as any).state === 'running'
      serverName = (s as any).server_name || s.server?.name || s.server?.addr || ''
      bytesSent = s.bytes_sent || 0
      bytesReceived = s.bytes_recv || 0
      apiError = false
    } catch {
      apiError = true
    }
  }

  async function toggle() {
    loading = true
    try {
      if (connected) {
        loadingStatus = ''
        await api.disconnect()
      } else {
        // Auto-select the lowest-latency server when multiple servers exist
        try {
          const data = await api.getServers()
          if (data.servers && data.servers.length > 1) {
            loadingStatus = t('simple.findingBestServer')
            const result = await api.autoSelectServer()
            serverName = result.server.name || result.server.addr
          }
        } catch {
          // If auto-select fails, proceed with current server
        }
        loadingStatus = ''
        await api.connect()
      }
      await refresh()
    } finally {
      loading = false
      loadingStatus = ''
    }
  }

  function formatSpeed(bytesPerSec: number): string {
    if (bytesPerSec < 1024) return `${bytesPerSec} B/s`
    if (bytesPerSec < 1024 * 1024) return `${(bytesPerSec / 1024).toFixed(1)} KB/s`
    return `${(bytesPerSec / 1024 / 1024).toFixed(1)} MB/s`
  }

  function formatBytes(bytes: number): string {
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
    return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`
  }
</script>

<div class="simple-mode">
  {#if apiError}
    <div class="error-banner">
      {t('app.backendError')}
      <button onclick={refresh}>{t('app.retry')}</button>
    </div>
  {/if}

  <div class="status-indicator" class:connected>
    <div class="dot"></div>
    <span>{connected ? t('simple.connected') : t('simple.disconnected')}</span>
  </div>

  {#if connected && serverName}
    <div class="server-info">{serverName}</div>
  {/if}

  <button
    class="connect-btn"
    class:connected
    onclick={toggle}
    disabled={loading}
  >
    {#if loading}
      <div class="btn-spinner"></div>
    {:else}
      {connected ? t('simple.disconnect') : t('simple.connect')}
    {/if}
  </button>

  {#if loadingStatus}
    <div class="loading-status">{loadingStatus}</div>
  {/if}

  {#if connected}
    <div class="speed-display">
      <div class="speed-item">
        <span class="speed-label">{t('simple.upload')}</span>
        <span class="speed-value">{formatSpeed(uploadSpeed)}</span>
      </div>
      <div class="speed-item">
        <span class="speed-label">{t('simple.download')}</span>
        <span class="speed-value">{formatSpeed(downloadSpeed)}</span>
      </div>
    </div>

    <div class="traffic-summary">
      <div>{t('simple.sent')}: {formatBytes(bytesSent)}</div>
      <div>{t('simple.received')}: {formatBytes(bytesReceived)}</div>
    </div>
  {/if}

  {#if onSwitchMode}
    <button class="mode-switch" onclick={onSwitchMode}>
      {t('simple.advancedMode')}
    </button>
  {/if}
</div>

<style>
  .simple-mode {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    min-height: 100vh;
    gap: 1.5rem;
    padding: 2rem;
    background: var(--bg-primary);
    color: var(--text-primary);
  }

  .error-banner {
    background: var(--accent-red-subtle);
    border: 1px solid var(--accent-red);
    color: var(--accent-red);
    padding: 8px 16px;
    border-radius: var(--radius-md);
    font-size: 13px;
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .error-banner button {
    background: var(--accent-red);
    color: #fff;
    border: none;
    border-radius: var(--radius-sm);
    padding: 4px 12px;
    cursor: pointer;
    font-size: 12px;
  }

  .status-indicator {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 1.1rem;
    color: var(--text-secondary);
  }

  .dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    background: var(--text-muted);
    transition: background 0.3s, box-shadow 0.3s;
  }

  .status-indicator.connected .dot {
    background: var(--accent-green);
    box-shadow: 0 0 8px var(--accent-green);
  }

  .server-info {
    font-size: 0.9rem;
    color: var(--text-muted);
  }

  .loading-status {
    font-size: 0.8rem;
    color: var(--text-muted);
    animation: pulse 1.5s ease-in-out infinite;
  }

  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.4; }
  }

  .connect-btn {
    width: 140px;
    height: 140px;
    border-radius: 50%;
    border: 3px solid var(--border);
    background: var(--bg-surface);
    color: var(--text-primary);
    font-size: 1.1rem;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.3s;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .connect-btn:hover:not(:disabled) {
    border-color: var(--accent-green);
    box-shadow: 0 0 20px rgba(52, 211, 153, 0.2);
  }

  .connect-btn.connected {
    border-color: var(--accent-green);
    box-shadow: 0 0 30px rgba(52, 211, 153, 0.15);
  }

  .connect-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .btn-spinner {
    width: 24px;
    height: 24px;
    border: 3px solid var(--border);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .speed-display {
    display: flex;
    gap: 3rem;
    font-size: 1.5rem;
    font-weight: 300;
  }

  .speed-item {
    display: flex;
    align-items: baseline;
    gap: 0.5rem;
  }

  .speed-label {
    font-size: 0.85rem;
    color: var(--text-muted);
  }

  .speed-value {
    font-variant-numeric: tabular-nums;
  }

  .traffic-summary {
    display: flex;
    gap: 2rem;
    font-size: 0.85rem;
    color: var(--text-muted);
  }

  .mode-switch {
    position: fixed;
    bottom: 2rem;
    right: 2rem;
    background: none;
    border: 1px solid var(--border);
    color: var(--text-secondary);
    padding: 0.5rem 1rem;
    border-radius: var(--radius-sm);
    cursor: pointer;
    font-size: 0.8rem;
    transition: border-color 0.15s, color 0.15s;
  }

  .mode-switch:hover {
    border-color: var(--border-light);
    color: var(--text-primary);
  }
</style>
