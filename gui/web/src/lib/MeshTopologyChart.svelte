<script lang="ts">
  import { onMount } from 'svelte'

  /** @type {{ virtual_ip: string, state: string, method: string, avg_rtt_ms: number }[]} */
  let { peers = [], selfIP = '', hubIP = '' } = $props()

  let canvas = $state(null)
  let width = $state(400)
  let height = $state(300)
  let hoveredNode = $state(null)
  let mousePos = $state({ x: 0, y: 0 })

  // Node positions
  let nodePositions = $state({})

  $effect(() => {
    if (canvas && peers) {
      calculatePositions()
      draw()
    }
  })

  function calculatePositions() {
    const centerX = width / 2
    const centerY = height / 2
    const hubRadius = 80
    const peerRadius = 120

    // Self node at center
    nodePositions.self = { x: centerX, y: centerY, type: 'self', ip: selfIP || 'You' }

    // Hub node above self
    nodePositions.hub = { x: centerX, y: centerY - hubRadius, type: 'hub', ip: hubIP || 'Hub' }

    // Peers in a circle around self
    const peerCount = peers.length
    peers.forEach((peer, i) => {
      const angle = (i / peerCount) * 2 * Math.PI - Math.PI / 2 + Math.PI / peerCount
      nodePositions[`peer_${i}`] = {
        x: centerX + Math.cos(angle) * peerRadius,
        y: centerY + Math.sin(angle) * peerRadius,
        type: 'peer',
        peer: peer,
        ip: peer.virtual_ip
      }
    })
  }

  function draw() {
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    ctx.clearRect(0, 0, width, height)

    // Draw connections first (behind nodes)
    drawConnections(ctx)

    // Draw nodes
    drawNode(ctx, nodePositions.hub)
    drawNode(ctx, nodePositions.self)
    peers.forEach((_, i) => {
      drawNode(ctx, nodePositions[`peer_${i}`])
    })
  }

  function drawConnections(ctx) {
    const self = nodePositions.self
    const hub = nodePositions.hub

    // Connection from self to hub (always exists)
    drawConnection(ctx, self, hub, 'relay', true)

    // Connections from self to peers
    peers.forEach((peer, i) => {
      const peerNode = nodePositions[`peer_${i}`]
      const method = peer.method || 'relay'
      const connected = peer.state === 'connected'

      if (method === 'p2p' || method === 'direct') {
        // Direct P2P connection
        drawConnection(ctx, self, peerNode, method, connected)
      } else {
        // Relay through hub
        drawConnection(ctx, self, hub, 'relay', true, 0.5)
        drawConnection(ctx, hub, peerNode, 'relay', connected)
      }
    })
  }

  function drawConnection(ctx, from, to, method, connected, alpha = 1) {
    ctx.save()
    ctx.globalAlpha = alpha

    // Line style based on method
    if (method === 'p2p' || method === 'direct') {
      ctx.strokeStyle = connected ? '#34d399' : '#55566a'
      ctx.setLineDash([])
    } else {
      ctx.strokeStyle = connected ? '#4f6df5' : '#55566a'
      ctx.setLineDash([5, 5])
    }

    ctx.lineWidth = connected ? 2 : 1
    ctx.beginPath()
    ctx.moveTo(from.x, from.y)
    ctx.lineTo(to.x, to.y)
    ctx.stroke()
    ctx.restore()
  }

  function drawNode(ctx, node) {
    if (!node) return
    const isHovered = hoveredNode === node

    ctx.save()

    // Node circle
    ctx.beginPath()
    ctx.arc(node.x, node.y, isHovered ? 22 : 20, 0, 2 * Math.PI)

    // Fill based on type
    if (node.type === 'self') {
      ctx.fillStyle = '#4f6df5'
      ctx.strokeStyle = '#3b57e0'
    } else if (node.type === 'hub') {
      ctx.fillStyle = '#9394a5'
      ctx.strokeStyle = '#55566a'
    } else {
      const peer = node.peer
      if (peer?.state === 'connected') {
        if (peer.method === 'p2p' || peer.method === 'direct') {
          ctx.fillStyle = '#34d399'
          ctx.strokeStyle = '#10b981'
        } else {
          ctx.fillStyle = '#4f6df5'
          ctx.strokeStyle = '#3b57e0'
        }
      } else if (peer?.state === 'connecting') {
        ctx.fillStyle = '#fbbf24'
        ctx.strokeStyle = '#f59e0b'
      } else {
        ctx.fillStyle = '#353549'
        ctx.strokeStyle = '#2a2a3d'
      }
    }

    ctx.lineWidth = 2
    ctx.fill()
    ctx.stroke()

    // Node icon
    ctx.fillStyle = '#fff'
    ctx.font = '12px system-ui'
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'

    if (node.type === 'self') {
      ctx.fillText('You', node.x, node.y)
    } else if (node.type === 'hub') {
      ctx.fillText('Hub', node.x, node.y)
    } else {
      // Peer - show method icon
      const peer = node.peer
      if (peer?.method === 'p2p' || peer?.method === 'direct') {
        ctx.fillText('P2P', node.x, node.y)
      } else {
        ctx.fillText('Relay', node.x, node.y)
      }
    }

    // Label below node
    ctx.fillStyle = '#9394a5'
    ctx.font = '10px monospace'
    let label = node.ip
    if (node.peer?.avg_rtt_ms) {
      label += ` (${node.peer.avg_rtt_ms}ms)`
    }
    ctx.fillText(label, node.x, node.y + 32)

    ctx.restore()
  }

  function handleMouseMove(e) {
    if (!canvas) return
    const rect = canvas.getBoundingClientRect()
    const x = e.clientX - rect.left
    const y = e.clientY - rect.top
    mousePos = { x, y }

    // Check if hovering over a node
    let found = null
    for (const key in nodePositions) {
      const node = nodePositions[key]
      if (node) {
        const dist = Math.sqrt((x - node.x) ** 2 + (y - node.y) ** 2)
        if (dist < 25) {
          found = node
          break
        }
      }
    }
    if (hoveredNode !== found) {
      hoveredNode = found
      draw()
    }
  }

  function handleMouseLeave() {
    if (hoveredNode) {
      hoveredNode = null
      draw()
    }
  }

  onMount(() => {
    if (canvas) {
      calculatePositions()
      draw()
    }
  })
</script>

<div class="topology-container">
  <canvas
    bind:this={canvas}
    {width}
    {height}
    onmousemove={handleMouseMove}
    onmouseleave={handleMouseLeave}
  ></canvas>

  {#if hoveredNode?.peer}
    <div
      class="tooltip"
      style="left: {mousePos.x + 10}px; top: {mousePos.y - 10}px"
    >
      <div class="tooltip-row">
        <span class="label">IP:</span>
        <span class="value">{hoveredNode.peer.virtual_ip}</span>
      </div>
      <div class="tooltip-row">
        <span class="label">State:</span>
        <span class="value state-{hoveredNode.peer.state}">{hoveredNode.peer.state}</span>
      </div>
      <div class="tooltip-row">
        <span class="label">Method:</span>
        <span class="value">{hoveredNode.peer.method || 'relay'}</span>
      </div>
      {#if hoveredNode.peer.avg_rtt_ms}
        <div class="tooltip-row">
          <span class="label">RTT:</span>
          <span class="value">{hoveredNode.peer.avg_rtt_ms}ms</span>
        </div>
      {/if}
    </div>
  {/if}

  <div class="legend">
    <div class="legend-item">
      <span class="legend-line p2p"></span>
      <span>P2P Direct</span>
    </div>
    <div class="legend-item">
      <span class="legend-line relay"></span>
      <span>Relay</span>
    </div>
  </div>
</div>

<style>
  .topology-container {
    position: relative;
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: 8px;
  }

  canvas {
    display: block;
    margin: 0 auto;
  }

  .tooltip {
    position: absolute;
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 8px 12px;
    font-size: 11px;
    pointer-events: none;
    z-index: 10;
    box-shadow: var(--shadow-lg);
  }

  .tooltip-row {
    display: flex;
    justify-content: space-between;
    gap: 12px;
    margin: 2px 0;
  }

  .tooltip .label {
    color: var(--text-secondary);
  }

  .tooltip .value {
    color: var(--text-primary);
    font-family: 'JetBrains Mono', monospace;
  }

  .tooltip .state-connected {
    color: var(--accent-green);
  }

  .tooltip .state-connecting {
    color: var(--accent-yellow);
  }

  .tooltip .state-disconnected {
    color: var(--accent-red);
  }

  .legend {
    display: flex;
    justify-content: center;
    gap: 16px;
    margin-top: 8px;
    padding-top: 8px;
    border-top: 1px solid var(--border);
  }

  .legend-item {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 11px;
    color: var(--text-secondary);
  }

  .legend-line {
    width: 24px;
    height: 2px;
    display: inline-block;
  }

  .legend-line.p2p {
    background: var(--accent-green);
  }

  .legend-line.relay {
    background: var(--accent);
    background: repeating-linear-gradient(
      90deg,
      var(--accent) 0px,
      var(--accent) 5px,
      transparent 5px,
      transparent 10px
    );
  }
</style>
