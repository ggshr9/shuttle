<script lang="ts">
  import { onMount } from 'svelte'

  // Props
  let { uploadData = [], downloadData = [], maxPoints = 60, height = 120 } = $props()

  let canvas = $state(null)
  let width = $state(400)

  // Reactive max value calculation
  let maxValue = $derived(() => {
    const allValues = [...uploadData, ...downloadData]
    if (allValues.length === 0) return 1024 * 1024 // Default 1 MB/s
    const max = Math.max(...allValues)
    return max > 0 ? max * 1.2 : 1024 * 1024 // 20% headroom
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

  // Draw chart when data or dimensions change
  $effect(() => {
    if (!canvas) return

    const ctx = canvas.getContext('2d')
    const dpr = window.devicePixelRatio || 1

    // Set canvas size for high DPI
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

    // Calculate point spacing
    const pointWidth = width / (maxPoints - 1)
    const max = maxValue()

    // Draw download area (green)
    if (downloadData.length > 1) {
      ctx.beginPath()
      ctx.moveTo(0, height)
      for (let i = 0; i < downloadData.length; i++) {
        const x = i * pointWidth
        const y = height - (downloadData[i] / max) * height
        ctx.lineTo(x, y)
      }
      ctx.lineTo((downloadData.length - 1) * pointWidth, height)
      ctx.closePath()
      ctx.fillStyle = 'rgba(52, 211, 153, 0.15)'
      ctx.fill()

      // Draw download line
      ctx.beginPath()
      ctx.moveTo(0, height - (downloadData[0] / max) * height)
      for (let i = 1; i < downloadData.length; i++) {
        const x = i * pointWidth
        const y = height - (downloadData[i] / max) * height
        ctx.lineTo(x, y)
      }
      ctx.strokeStyle = '#34d399'
      ctx.lineWidth = 2
      ctx.stroke()
    }

    // Draw upload area (blue)
    if (uploadData.length > 1) {
      ctx.beginPath()
      ctx.moveTo(0, height)
      for (let i = 0; i < uploadData.length; i++) {
        const x = i * pointWidth
        const y = height - (uploadData[i] / max) * height
        ctx.lineTo(x, y)
      }
      ctx.lineTo((uploadData.length - 1) * pointWidth, height)
      ctx.closePath()
      ctx.fillStyle = 'rgba(79, 109, 245, 0.15)'
      ctx.fill()

      // Draw upload line
      ctx.beginPath()
      ctx.moveTo(0, height - (uploadData[0] / max) * height)
      for (let i = 1; i < uploadData.length; i++) {
        const x = i * pointWidth
        const y = height - (uploadData[i] / max) * height
        ctx.lineTo(x, y)
      }
      ctx.strokeStyle = '#4f6df5'
      ctx.lineWidth = 2
      ctx.stroke()
    }
  })

  function formatSpeed(bytes) {
    if (bytes < 1024) return bytes.toFixed(0) + ' B/s'
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB/s'
    return (bytes / 1024 / 1024).toFixed(1) + ' MB/s'
  }
</script>

<div class="chart-container">
  <canvas bind:this={canvas} style="width: 100%; height: {height}px;"></canvas>
  <div class="chart-labels">
    <span class="max-label">{formatSpeed(maxValue())}</span>
    <span class="min-label">0</span>
  </div>
  <div class="time-labels">
    <span>5m ago</span>
    <span>now</span>
  </div>
</div>

<style>
  .chart-container {
    position: relative;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    padding: 12px;
  }

  canvas {
    display: block;
  }

  .chart-labels {
    position: absolute;
    top: 12px;
    right: 16px;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    height: calc(100% - 44px);
    pointer-events: none;
  }

  .max-label, .min-label {
    font-size: 10px;
    color: var(--text-muted);
    background: var(--bg-secondary);
    padding: 2px 6px;
    border-radius: 4px;
  }

  .time-labels {
    display: flex;
    justify-content: space-between;
    margin-top: 4px;
    font-size: 10px;
    color: var(--text-muted);
  }
</style>
