<script lang="ts">
  import { onMount } from 'svelte'

  interface Peer {
    virtual_ip: string
    state: string
    method?: string
    avg_rtt_ms?: number
  }

  interface Props {
    peers?: Peer[]
    selfIP?: string
    hubIP?: string
  }

  let { peers = [], selfIP = '', hubIP = '' }: Props = $props()

  let canvas = $state<HTMLCanvasElement | null>(null)
  let width = $state(400)
  let height = $state(300)
  let hoveredNode = $state<any>(null)
  let mousePos = $state({ x: 0, y: 0 })

  let nodePositions = $state<Record<string, any>>({})

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

    nodePositions.self = { x: centerX, y: centerY, type: 'self', ip: selfIP || 'You' }
    nodePositions.hub = { x: centerX, y: centerY - hubRadius, type: 'hub', ip: hubIP || 'Hub' }

    const peerCount = peers.length
    peers.forEach((peer, i) => {
      const angle = (i / peerCount) * 2 * Math.PI - Math.PI / 2 + Math.PI / peerCount
      nodePositions[`peer_${i}`] = {
        x: centerX + Math.cos(angle) * peerRadius,
        y: centerY + Math.sin(angle) * peerRadius,
        type: 'peer',
        peer,
        ip: peer.virtual_ip,
      }
    })
  }

  function draw() {
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return
    ctx.clearRect(0, 0, width, height)

    drawConnections(ctx)
    drawNode(ctx, nodePositions.hub)
    drawNode(ctx, nodePositions.self)
    peers.forEach((_, i) => drawNode(ctx, nodePositions[`peer_${i}`]))
  }

  function drawConnections(ctx: CanvasRenderingContext2D) {
    const self = nodePositions.self
    const hub = nodePositions.hub

    drawConnection(ctx, self, hub, 'relay', true)

    peers.forEach((peer, i) => {
      const peerNode = nodePositions[`peer_${i}`]
      const method = peer.method || 'relay'
      const connected = peer.state === 'connected'

      if (method === 'p2p' || method === 'direct') {
        drawConnection(ctx, self, peerNode, method, connected)
      } else {
        drawConnection(ctx, self, hub, 'relay', true, 0.5)
        drawConnection(ctx, hub, peerNode, 'relay', connected)
      }
    })
  }

  function drawConnection(
    ctx: CanvasRenderingContext2D,
    from: any, to: any, method: string, connected: boolean, alpha = 1,
  ) {
    ctx.save()
    ctx.globalAlpha = alpha

    if (method === 'p2p' || method === 'direct') {
      ctx.strokeStyle = connected ? '#22c55e' : '#52525b'
      ctx.setLineDash([])
    } else {
      ctx.strokeStyle = connected ? '#3b82f6' : '#52525b'
      ctx.setLineDash([5, 5])
    }

    ctx.lineWidth = connected ? 2 : 1
    ctx.beginPath()
    ctx.moveTo(from.x, from.y)
    ctx.lineTo(to.x, to.y)
    ctx.stroke()
    ctx.restore()
  }

  function drawNode(ctx: CanvasRenderingContext2D, node: any) {
    if (!node) return
    const isHovered = hoveredNode === node

    ctx.save()

    ctx.beginPath()
    ctx.arc(node.x, node.y, isHovered ? 22 : 20, 0, 2 * Math.PI)

    if (node.type === 'self') {
      ctx.fillStyle = '#3b82f6'
      ctx.strokeStyle = '#2563eb'
    } else if (node.type === 'hub') {
      ctx.fillStyle = '#a1a1aa'
      ctx.strokeStyle = '#52525b'
    } else {
      const peer = node.peer
      if (peer?.state === 'connected') {
        if (peer.method === 'p2p' || peer.method === 'direct') {
          ctx.fillStyle = '#22c55e'
          ctx.strokeStyle = '#16a34a'
        } else {
          ctx.fillStyle = '#3b82f6'
          ctx.strokeStyle = '#2563eb'
        }
      } else if (peer?.state === 'connecting') {
        ctx.fillStyle = '#eab308'
        ctx.strokeStyle = '#ca8a04'
      } else {
        ctx.fillStyle = '#27272a'
        ctx.strokeStyle = '#3f3f46'
      }
    }

    ctx.lineWidth = 2
    ctx.fill()
    ctx.stroke()

    ctx.fillStyle = '#fff'
    ctx.font = '12px system-ui'
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'

    if (node.type === 'self') {
      ctx.fillText('You', node.x, node.y)
    } else if (node.type === 'hub') {
      ctx.fillText('Hub', node.x, node.y)
    } else {
      const peer = node.peer
      if (peer?.method === 'p2p' || peer?.method === 'direct') {
        ctx.fillText('P2P', node.x, node.y)
      } else {
        ctx.fillText('Relay', node.x, node.y)
      }
    }

    ctx.fillStyle = '#a1a1aa'
    ctx.font = '10px monospace'
    let label = node.ip
    if (node.peer?.avg_rtt_ms) {
      label += ` (${node.peer.avg_rtt_ms}ms)`
    }
    ctx.fillText(label, node.x, node.y + 32)

    ctx.restore()
  }

  function handleMouseMove(e: MouseEvent) {
    if (!canvas) return
    const rect = canvas.getBoundingClientRect()
    const x = e.clientX - rect.left
    const y = e.clientY - rect.top
    mousePos = { x, y }

    let found: any = null
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
      <span>P2P</span>
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
    background: var(--shuttle-bg-surface);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    padding: var(--shuttle-space-2);
  }

  canvas {
    display: block;
    margin: 0 auto;
  }

  .tooltip {
    position: absolute;
    background: var(--shuttle-bg-subtle);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-sm);
    padding: var(--shuttle-space-2) var(--shuttle-space-3);
    font-size: var(--shuttle-text-xs);
    pointer-events: none;
    z-index: 10;
    box-shadow: var(--shuttle-shadow-md);
  }

  .tooltip-row {
    display: flex;
    justify-content: space-between;
    gap: var(--shuttle-space-3);
    margin: 2px 0;
  }

  .tooltip .label { color: var(--shuttle-fg-secondary); }

  .tooltip .value {
    color: var(--shuttle-fg-primary);
    font-family: var(--shuttle-font-mono);
  }

  .tooltip .state-connected    { color: var(--shuttle-success); }
  .tooltip .state-connecting   { color: var(--shuttle-warning); }
  .tooltip .state-disconnected { color: var(--shuttle-danger); }

  .legend {
    display: flex;
    justify-content: center;
    gap: var(--shuttle-space-4);
    margin-top: var(--shuttle-space-2);
    padding-top: var(--shuttle-space-2);
    border-top: 1px solid var(--shuttle-border);
  }

  .legend-item {
    display: flex;
    align-items: center;
    gap: var(--shuttle-space-1);
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-secondary);
  }

  .legend-line {
    width: 24px;
    height: 2px;
    display: inline-block;
  }

  .legend-line.p2p { background: var(--shuttle-success); }

  .legend-line.relay {
    background: repeating-linear-gradient(
      90deg,
      var(--shuttle-info) 0px,
      var(--shuttle-info) 5px,
      transparent 5px,
      transparent 10px
    );
  }
</style>
