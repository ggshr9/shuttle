<script lang="ts">
  import { onMount } from 'svelte'

  // Props
  let { peers = [], height = 200 } = $props()

  let canvas = $state(null)
  let width = $state(400)

  // History for RTT values (store last 60 samples per peer)
  let rttHistory = $state({})
  const maxPoints = 60

  // Update history when peers change
  $effect(() => {
    for (const peer of peers) {
      if (!rttHistory[peer.virtual_ip]) {
        rttHistory[peer.virtual_ip] = []
      }
      const history = rttHistory[peer.virtual_ip]
      history.push(peer.avg_rtt_ms || 0)
      if (history.length > maxPoints) {
        history.shift()
      }
    }
  })

  onMount(() => {
    const resizeObserver = new ResizeObserver(entries => {
      for (const entry of entries) {
        width = entry.contentRect.width
      }
    })
    if (canvas?.parentElement) {
      resizeObserver.observe(canvas.parentElement)
    }
    return () => resizeObserver.disconnect()
  })

  // Color palette for different peers
  const colors = [
    { line: '#34d399', fill: 'rgba(52, 211, 153, 0.15)' },
    { line: '#4f6df5', fill: 'rgba(79, 109, 245, 0.15)' },
    { line: '#f59e0b', fill: 'rgba(245, 158, 11, 0.15)' },
    { line: '#a78bfa', fill: 'rgba(167, 139, 250, 0.15)' },
    { line: '#ec4899', fill: 'rgba(236, 72, 153, 0.15)' },
  ]

  // Calculate max RTT for scaling
  let maxRTT = $derived(() => {
    let max = 100 // minimum 100ms scale
    for (const history of Object.values(rttHistory)) {
      for (const rtt of history) {
        if (rtt > max) max = rtt
      }
    }
    return max * 1.2 // 20% headroom
  })

  // Draw chart
  $effect(() => {
    if (!canvas) return

    const ctx = canvas.getContext('2d')
    const dpr = window.devicePixelRatio || 1

    canvas.width = width * dpr
    canvas.height = height * dpr
    ctx.scale(dpr, dpr)

    // Clear canvas
    ctx.clearRect(0, 0, width, height)

    // Draw background grid
    ctx.strokeStyle = '#1e1e2e'
    ctx.lineWidth = 1
    const gridLines = 4
    for (let i = 1; i < gridLines; i++) {
      const y = (height / gridLines) * i
      ctx.beginPath()
      ctx.moveTo(0, y)
      ctx.lineTo(width, y)
      ctx.stroke()
    }

    const pointWidth = width / (maxPoints - 1)
    const max = maxRTT()

    // Draw each peer's RTT history
    let colorIndex = 0
    for (const [vip, history] of Object.entries(rttHistory)) {
      if (history.length < 2) continue

      const color = colors[colorIndex % colors.length]
      colorIndex++

      // Draw filled area
      ctx.beginPath()
      ctx.moveTo(0, height)
      for (let i = 0; i < history.length; i++) {
        const x = i * pointWidth
        const y = height - (history[i] / max) * height
        ctx.lineTo(x, y)
      }
      ctx.lineTo((history.length - 1) * pointWidth, height)
      ctx.closePath()
      ctx.fillStyle = color.fill
      ctx.fill()

      // Draw line
      ctx.beginPath()
      ctx.moveTo(0, height - (history[0] / max) * height)
      for (let i = 1; i < history.length; i++) {
        const x = i * pointWidth
        const y = height - (history[i] / max) * height
        ctx.lineTo(x, y)
      }
      ctx.strokeStyle = color.line
      ctx.lineWidth = 2
      ctx.stroke()
    }
  })

  function formatRTT(ms) {
    if (ms < 1) return '<1ms'
    return Math.round(ms) + 'ms'
  }

  function getScoreColor(score) {
    if (score >= 80) return '#34d399'
    if (score >= 50) return '#f59e0b'
    return '#f87171'
  }

  function getStateIcon(state) {
    switch (state) {
      case 'connected': return '●'
      case 'connecting': return '◐'
      default: return '○'
    }
  }
</script>

<div class="quality-container">
  <div class="chart-section">
    <h4>RTT History</h4>
    <div class="chart-wrapper">
      <canvas bind:this={canvas} style="width: 100%; height: {height}px;"></canvas>
      <div class="chart-labels">
        <span class="max-label">{formatRTT(maxRTT())}</span>
        <span class="min-label">0ms</span>
      </div>
    </div>
  </div>

  {#if peers.length > 0}
    <div class="peers-section">
      <h4>Mesh Peers</h4>
      <div class="peers-list">
        {#each peers as peer, i}
          <div class="peer-item">
            <div class="peer-header">
              <span class="peer-state" style="color: {peer.state === 'connected' ? '#34d399' : '#55566a'}">
                {getStateIcon(peer.state)}
              </span>
              <span class="peer-ip">{peer.virtual_ip}</span>
              {#if peer.method}
                <span class="peer-method">{peer.method}</span>
              {/if}
            </div>
            <div class="peer-stats">
              <div class="stat">
                <span class="stat-label">RTT</span>
                <span class="stat-value">{formatRTT(peer.avg_rtt_ms)}</span>
              </div>
              <div class="stat">
                <span class="stat-label">Jitter</span>
                <span class="stat-value">{formatRTT(peer.jitter_ms)}</span>
              </div>
              <div class="stat">
                <span class="stat-label">Loss</span>
                <span class="stat-value">{(peer.packet_loss * 100).toFixed(1)}%</span>
              </div>
              <div class="stat">
                <span class="stat-label">Score</span>
                <span class="stat-value" style="color: {getScoreColor(peer.score)}">{peer.score}</span>
              </div>
            </div>
            <div class="peer-legend" style="background: {colors[i % colors.length].line}"></div>
          </div>
        {/each}
      </div>
    </div>
  {:else}
    <div class="no-peers">
      <span>No mesh peers connected</span>
    </div>
  {/if}
</div>

<style>
  .quality-container {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: 16px;
  }

  h4 {
    margin: 0 0 12px 0;
    font-size: 14px;
    font-weight: 600;
    color: var(--text-primary);
  }

  .chart-wrapper {
    position: relative;
    background: var(--bg-surface);
    border-radius: var(--radius-sm);
    padding: 8px;
  }

  canvas {
    display: block;
  }

  .chart-labels {
    position: absolute;
    top: 8px;
    right: 12px;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    height: calc(100% - 16px);
    pointer-events: none;
  }

  .max-label, .min-label {
    font-size: 10px;
    color: var(--text-muted);
    background: var(--bg-secondary);
    padding: 2px 6px;
    border-radius: 4px;
  }

  .peers-section {
    margin-top: 16px;
  }

  .peers-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .peer-item {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 10px;
    background: var(--bg-surface);
    border-radius: var(--radius-sm);
    position: relative;
  }

  .peer-legend {
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    width: 3px;
    border-radius: var(--radius-sm) 0 0 var(--radius-sm);
  }

  .peer-header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding-left: 8px;
  }

  .peer-state {
    font-size: 10px;
  }

  .peer-ip {
    font-family: 'JetBrains Mono', monospace;
    font-size: 13px;
    color: var(--text-primary);
  }

  .peer-method {
    font-size: 11px;
    color: var(--text-muted);
    background: var(--bg-tertiary);
    padding: 2px 8px;
    border-radius: 4px;
    font-weight: 500;
  }

  .peer-stats {
    display: flex;
    gap: 16px;
    padding-left: 8px;
  }

  .stat {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .stat-label {
    font-size: 10px;
    color: var(--text-muted);
    text-transform: uppercase;
    font-weight: 500;
  }

  .stat-value {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-primary);
    font-variant-numeric: tabular-nums;
  }

  .no-peers {
    text-align: center;
    padding: 24px;
    color: var(--text-muted);
    font-size: 13px;
  }
</style>
