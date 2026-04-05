<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { api } from '../lib/api'
  import { toast } from '../lib/toast'
  import { t } from '../lib/i18n/index'
  import MeshTopologyChart from '../lib/MeshTopologyChart.svelte'

  let status = $state<import('../lib/api').MeshStatus | null>(null)
  let peers = $state<import('../lib/api').MeshPeer[]>([])
  let connecting = $state<Record<string, boolean>>({})
  let loading = $state(true)
  let pollInterval: ReturnType<typeof setInterval> | null = null

  async function loadStatus() {
    try {
      status = await api.meshStatus()
    } catch {
      // mesh may not be enabled — ignore silently
    }
  }

  async function loadPeers() {
    try {
      peers = await api.meshPeers()
    } catch {
      // ignore
    }
  }

  async function load() {
    loading = true
    await Promise.all([loadStatus(), loadPeers()])
    loading = false
  }

  async function connectPeer(vip: string) {
    connecting = { ...connecting, [vip]: true }
    try {
      await api.meshConnectPeer(vip)
      toast.success(t('mesh.connectSuccess', { vip }))
      await loadPeers()
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      const next = { ...connecting }
      delete next[vip]
      connecting = next
    }
  }

  function stateClass(state: string): string {
    if (state === 'connected') return 'state-connected'
    if (state === 'connecting') return 'state-connecting'
    return 'state-disconnected'
  }

  function formatRTT(ms: number | undefined): string {
    if (!ms) return '—'
    return `${ms}ms`
  }

  function formatLoss(rate: number | undefined): string {
    if (rate === undefined || rate === null) return '—'
    return `${(rate * 100).toFixed(1)}%`
  }

  // Derive selfIP and hubIP from status for the topology chart
  let selfIP = $derived(status?.vip ?? '')
  let hubIP = $derived(status?.hub ?? '')

  onMount(() => {
    load()
    pollInterval = setInterval(loadPeers, 5000)
  })

  onDestroy(() => {
    if (pollInterval) clearInterval(pollInterval)
  })
</script>

<div class="page">
  <div class="page-header">
    <h2>{t('mesh.title')}</h2>
  </div>

  <!-- Status Card -->
  <div class="status-card">
    {#if loading && !status}
      <div class="loading-inline">{t('common.loading')}</div>
    {:else if !status}
      <div class="mesh-disabled">
        <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="5" cy="10" r="2"/>
          <circle cx="15" cy="5" r="2"/>
          <circle cx="15" cy="15" r="2"/>
          <path d="M7 10h3l2-5h1M10 10l2 5h1"/>
        </svg>
        <span>{t('mesh.notEnabled')}</span>
      </div>
    {:else}
      <div class="status-grid">
        <div class="status-item">
          <span class="status-label">{t('mesh.status')}</span>
          <span class="status-value">
            <span class="dot {status.enabled ? 'dot-green' : 'dot-gray'}"></span>
            {status.enabled ? t('mesh.enabled') : t('mesh.disabled')}
          </span>
        </div>
        <div class="status-item">
          <span class="status-label">{t('mesh.virtualIP')}</span>
          <span class="status-value mono">{status.virtual_ip || '—'}</span>
        </div>
        <div class="status-item">
          <span class="status-label">{t('mesh.cidr')}</span>
          <span class="status-value mono">{status.cidr || '—'}</span>
        </div>
        <div class="status-item">
          <span class="status-label">{t('mesh.peers')}</span>
          <span class="status-value">{peers.length}</span>
        </div>
      </div>
    {/if}
  </div>

  <!-- Peer Table -->
  <div class="section-header">
    <h3>{t('mesh.peersTitle')}</h3>
  </div>

  {#if peers.length === 0 && !loading}
    <div class="empty-state">
      <svg width="40" height="40" viewBox="0 0 40 40" fill="none" stroke="var(--text-muted)" stroke-width="1.5">
        <circle cx="10" cy="20" r="4"/>
        <circle cx="30" cy="10" r="4"/>
        <circle cx="30" cy="30" r="4"/>
        <path d="M14 20h6l6-10h1M20 20l6 10h1"/>
      </svg>
      <p>{t('mesh.noPeers')}</p>
    </div>
  {:else}
    <div class="table-wrapper">
      <table class="peer-table">
        <thead>
          <tr>
            <th>{t('mesh.col.vip')}</th>
            <th>{t('mesh.col.state')}</th>
            <th>{t('mesh.col.method')}</th>
            <th>{t('mesh.col.rtt')}</th>
            <th>{t('mesh.col.loss')}</th>
            <th>{t('mesh.col.score')}</th>
            <th>{t('mesh.col.action')}</th>
          </tr>
        </thead>
        <tbody>
          {#each peers as peer}
            <tr>
              <td class="mono">{peer.virtual_ip}</td>
              <td>
                <span class="state-badge {stateClass(peer.state)}">
                  {peer.state}
                </span>
              </td>
              <td class="method">{peer.method || 'relay'}</td>
              <td class="mono">{formatRTT(peer.avg_rtt_ms)}</td>
              <td class="mono">{formatLoss(peer.packet_loss)}</td>
              <td class="mono">{peer.score != null ? peer.score.toFixed(1) : '—'}</td>
              <td>
                {#if peer.state !== 'connected'}
                  <button
                    class="btn-connect"
                    onclick={() => connectPeer(peer.virtual_ip)}
                    disabled={!!connecting[peer.virtual_ip]}
                  >
                    {connecting[peer.virtual_ip] ? t('mesh.connecting') : t('mesh.connect')}
                  </button>
                {:else}
                  <span class="connected-label">{t('mesh.connected')}</span>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}

  <!-- Topology Chart -->
  <div class="section-header" style="margin-top: 28px;">
    <h3>{t('mesh.topology')}</h3>
  </div>
  <MeshTopologyChart {peers} {selfIP} {hubIP} />
</div>

<style>
  .page { max-width: 820px; }

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
    margin: 0;
  }

  /* Status card */
  .status-card {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: 18px 20px;
    margin-bottom: 24px;
  }

  .loading-inline {
    font-size: 13px;
    color: var(--text-muted);
  }

  .mesh-disabled {
    display: flex;
    align-items: center;
    gap: 10px;
    color: var(--text-muted);
    font-size: 14px;
  }

  .status-grid {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 16px;
  }

  @media (max-width: 640px) {
    .status-grid { grid-template-columns: repeat(2, 1fr); }
  }

  .status-item {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .status-label {
    font-size: 11px;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    font-weight: 500;
  }

  .status-value {
    font-size: 14px;
    font-weight: 500;
    color: var(--text-primary);
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .mono {
    font-family: 'JetBrains Mono', 'Fira Code', monospace;
    font-size: 13px;
  }

  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .dot-green { background: var(--accent-green); }
  .dot-gray  { background: var(--text-muted); }

  /* Section header */
  .section-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 12px;
  }

  /* Empty state */
  .empty-state {
    text-align: center;
    padding: 40px 24px;
    background: var(--bg-secondary);
    border: 1px dashed var(--border);
    border-radius: var(--radius-lg);
    color: var(--text-secondary);
  }

  .empty-state p {
    margin: 12px 0 0;
    font-size: 14px;
  }

  /* Peer table */
  .table-wrapper {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    overflow: hidden;
  }

  .peer-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;
  }

  .peer-table thead {
    background: var(--bg-surface);
  }

  .peer-table th {
    text-align: left;
    padding: 10px 14px;
    font-size: 11px;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    border-bottom: 1px solid var(--border);
    white-space: nowrap;
  }

  .peer-table td {
    padding: 11px 14px;
    color: var(--text-primary);
    border-bottom: 1px solid var(--border);
    vertical-align: middle;
  }

  .peer-table tbody tr:last-child td {
    border-bottom: none;
  }

  .peer-table tbody tr:hover {
    background: var(--bg-hover);
  }

  /* State badge */
  .state-badge {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 2px 9px;
    border-radius: 10px;
    font-size: 11px;
    font-weight: 500;
    text-transform: capitalize;
  }

  .state-connected {
    background: var(--accent-green-subtle);
    color: var(--accent-green);
  }

  .state-connecting {
    background: var(--accent-yellow-subtle);
    color: var(--accent-yellow);
  }

  .state-disconnected {
    background: var(--accent-red-subtle);
    color: var(--accent-red);
  }

  .method {
    color: var(--text-secondary);
    text-transform: capitalize;
  }

  /* Connect button */
  .btn-connect {
    background: var(--accent-subtle);
    color: var(--accent);
    border: 1px solid transparent;
    border-radius: var(--radius-sm);
    padding: 4px 12px;
    font-size: 12px;
    font-weight: 500;
    font-family: inherit;
    cursor: pointer;
    transition: all 0.15s;
    white-space: nowrap;
  }

  .btn-connect:hover:not(:disabled) {
    background: var(--accent);
    color: #fff;
  }

  .btn-connect:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .connected-label {
    font-size: 12px;
    color: var(--accent-green);
    font-weight: 500;
  }
</style>
