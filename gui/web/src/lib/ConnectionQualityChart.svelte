<script>
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
    { line: '#3fb950', fill: 'rgba(63, 185, 80, 0.2)' },
    { line: '#58a6ff', fill: 'rgba(88, 166, 255, 0.2)' },
    { line: '#f0883e', fill: 'rgba(240, 136, 62, 0.2)' },
    { line: '#a371f7', fill: 'rgba(163, 113, 247, 0.2)' },
    { line: '#db61a2', fill: 'rgba(219, 97, 162, 0.2)' },
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
    ctx.strokeStyle = '#21262d'
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
    if (score >= 80) return '#3fb950'
    if (score >= 50) return '#f0883e'
    return '#f85149'
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
              <span class="peer-state" style="color: {peer.state === 'connected' ? '#3fb950' : '#6e7681'}">
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
    background: #161b22;
    border: 1px solid #2d333b;
    border-radius: 8px;
    padding: 16px;
  }

  h4 {
    margin: 0 0 12px 0;
    font-size: 14px;
    font-weight: 500;
    color: #c9d1d9;
  }

  .chart-wrapper {
    position: relative;
    background: #0d1117;
    border-radius: 6px;
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
    color: #6e7681;
    background: rgba(13, 17, 23, 0.8);
    padding: 2px 4px;
    border-radius: 2px;
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
    background: #0d1117;
    border-radius: 6px;
    position: relative;
  }

  .peer-legend {
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    width: 3px;
    border-radius: 6px 0 0 6px;
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
    font-family: monospace;
    font-size: 13px;
    color: #c9d1d9;
  }

  .peer-method {
    font-size: 11px;
    color: #6e7681;
    background: #21262d;
    padding: 2px 6px;
    border-radius: 4px;
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
    color: #6e7681;
    text-transform: uppercase;
  }

  .stat-value {
    font-size: 13px;
    font-weight: 500;
    color: #c9d1d9;
  }

  .no-peers {
    text-align: center;
    padding: 24px;
    color: #6e7681;
    font-size: 13px;
  }
</style>
