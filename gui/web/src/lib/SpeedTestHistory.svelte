<script lang="ts">
  import { api, type SpeedtestHistoryEntry } from './api'
  import { onMount } from 'svelte'
  import { t } from './i18n/index'

  let entries: SpeedtestHistoryEntry[] = $state([])
  let loading = $state(false)
  let days = $state(30)

  // Unique server addresses for color coding
  let servers = $derived([...new Set(entries.map(e => e.server_addr))])

  // Color palette for servers
  const palette = [
    'var(--accent)',
    'var(--accent-green)',
    '#e5a00d',
    '#e06c75',
    '#56b6c2',
    '#c678dd',
    '#d19a66',
    '#98c379',
  ]

  function serverColor(addr: string): string {
    const idx = servers.indexOf(addr)
    return palette[idx % palette.length]
  }

  function serverLabel(entry: SpeedtestHistoryEntry): string {
    return entry.server_name || entry.server_addr
  }

  // Stats
  let availableEntries = $derived(entries.filter(e => e.available))
  let avgLatency = $derived(
    availableEntries.length > 0
      ? Math.round(availableEntries.reduce((s, e) => s + e.latency_ms, 0) / availableEntries.length)
      : 0
  )
  let bestLatency = $derived(
    availableEntries.length > 0
      ? Math.min(...availableEntries.map(e => e.latency_ms))
      : 0
  )
  let worstLatency = $derived(
    availableEntries.length > 0
      ? Math.max(...availableEntries.map(e => e.latency_ms))
      : 0
  )
  let availability = $derived(
    entries.length > 0
      ? Math.round((availableEntries.length / entries.length) * 100)
      : 0
  )

  // Chart data: group by timestamp (rounded to nearest minute) for plotting
  let maxLatency = $derived(
    availableEntries.length > 0
      ? Math.max(...availableEntries.map(e => e.latency_ms))
      : 100
  )

  const CHART_HEIGHT = 160
  const CHART_PADDING = 24

  // Build SVG path points per server
  function getServerPoints(addr: string): { x: number; y: number; entry: SpeedtestHistoryEntry }[] {
    const serverEntries = entries
      .filter(e => e.server_addr === addr && e.available)
      .sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime())

    if (serverEntries.length === 0 || entries.length === 0) return []

    const allTimes = entries.map(e => new Date(e.timestamp).getTime())
    const minTime = Math.min(...allTimes)
    const maxTime = Math.max(...allTimes)
    const timeRange = maxTime - minTime || 1

    return serverEntries.map(e => {
      const time = new Date(e.timestamp).getTime()
      const x = CHART_PADDING + ((time - minTime) / timeRange) * (100 - CHART_PADDING * 2 / 100 * 100)
      const y = CHART_HEIGHT - CHART_PADDING - ((e.latency_ms / (maxLatency * 1.1)) * (CHART_HEIGHT - CHART_PADDING * 2))
      return { x: (time - minTime) / timeRange * 100, y, entry: e }
    })
  }

  function buildPath(points: { x: number; y: number }[]): string {
    if (points.length === 0) return ''
    return points.map((p, i) => `${i === 0 ? 'M' : 'L'} ${p.x} ${p.y}`).join(' ')
  }

  async function load() {
    loading = true
    try {
      entries = await api.getSpeedtestHistory(days)
    } catch {
      entries = []
    } finally {
      loading = false
    }
  }

  function selectRange(d: number) {
    days = d
    load()
  }

  onMount(() => {
    load()
  })
</script>

<div class="speed-history">
  <div class="header">
    <h3>{t('speedtest.history')}</h3>
    <div class="range-selector">
      <button class:active={days === 7} onclick={() => selectRange(7)}>{t('speedtest.days7')}</button>
      <button class:active={days === 30} onclick={() => selectRange(30)}>{t('speedtest.days30')}</button>
      <button class:active={days === 90} onclick={() => selectRange(90)}>{t('speedtest.days90')}</button>
    </div>
  </div>

  {#if loading}
    <p class="empty">{t('common.loading')}</p>
  {:else if entries.length === 0}
    <p class="empty">{t('speedtest.noHistory')}</p>
  {:else}
    <div class="summary">
      <div class="stat-card">
        <span class="stat-label">{t('speedtest.avgLatency')}</span>
        <span class="stat-value">{avgLatency} ms</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">{t('speedtest.bestLatency')}</span>
        <span class="stat-value best">{bestLatency} ms</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">{t('speedtest.worstLatency')}</span>
        <span class="stat-value worst">{worstLatency} ms</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">{t('speedtest.availability')}</span>
        <span class="stat-value" class:good={availability >= 90} class:warn={availability < 90 && availability >= 70} class:bad={availability < 70}>{availability}%</span>
      </div>
    </div>

    <div class="chart-container">
      <svg viewBox="0 0 100 {CHART_HEIGHT}" preserveAspectRatio="none" class="chart-svg">
        <!-- Grid lines -->
        {#each [0.25, 0.5, 0.75] as frac}
          <line
            x1="0" y1={CHART_HEIGHT - CHART_PADDING - frac * (CHART_HEIGHT - CHART_PADDING * 2)}
            x2="100" y2={CHART_HEIGHT - CHART_PADDING - frac * (CHART_HEIGHT - CHART_PADDING * 2)}
            class="grid-line"
          />
        {/each}

        <!-- Lines per server -->
        {#each servers as addr}
          {@const points = getServerPoints(addr)}
          {#if points.length > 1}
            <polyline
              points={points.map(p => `${p.x},${p.y}`).join(' ')}
              fill="none"
              stroke={serverColor(addr)}
              stroke-width="0.5"
              vector-effect="non-scaling-stroke"
            />
          {/if}
          {#each points as point}
            <circle
              cx={point.x}
              cy={point.y}
              r="0.8"
              fill={serverColor(addr)}
              vector-effect="non-scaling-stroke"
            >
              <title>{serverLabel(point.entry)}: {point.entry.latency_ms}ms</title>
            </circle>
          {/each}
        {/each}
      </svg>

      <!-- Y-axis labels -->
      <div class="y-labels">
        <span>{Math.round(maxLatency * 1.1)} ms</span>
        <span>{Math.round(maxLatency * 1.1 * 0.5)} ms</span>
        <span>0 ms</span>
      </div>
    </div>

    <!-- Legend -->
    {#if servers.length > 1}
      <div class="legend">
        {#each servers as addr}
          <span class="legend-item">
            <span class="legend-dot" style="background: {serverColor(addr)}"></span>
            {entries.find(e => e.server_addr === addr)?.server_name || addr}
          </span>
        {/each}
      </div>
    {/if}
  {/if}
</div>

<style>
  .speed-history {
    margin: 20px 0;
  }

  .header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 12px;
  }

  .header h3 {
    margin: 0;
    font-size: 14px;
    color: var(--text-secondary);
    text-align: left;
  }

  .range-selector {
    display: flex;
    gap: 4px;
  }

  .range-selector button {
    padding: 4px 10px;
    font-size: 11px;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg-secondary);
    color: var(--text-secondary);
    cursor: pointer;
    transition: all 0.2s;
  }

  .range-selector button:hover {
    border-color: var(--accent);
  }

  .range-selector button.active {
    background: var(--accent);
    color: #fff;
    border-color: var(--accent);
  }

  .empty {
    text-align: center;
    color: var(--text-muted);
    font-size: 13px;
    padding: 24px 0;
  }

  .summary {
    display: flex;
    gap: 10px;
    margin-bottom: 16px;
  }

  .stat-card {
    flex: 1;
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 10px 12px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .stat-label {
    font-size: 11px;
    color: var(--text-secondary);
  }

  .stat-value {
    font-size: 16px;
    font-weight: 600;
    color: var(--text-primary);
  }

  .stat-value.best {
    color: var(--accent-green);
  }

  .stat-value.worst {
    color: #e06c75;
  }

  .stat-value.good {
    color: var(--accent-green);
  }

  .stat-value.warn {
    color: #e5a00d;
  }

  .stat-value.bad {
    color: #e06c75;
  }

  .chart-container {
    position: relative;
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 12px;
    height: 180px;
  }

  .chart-svg {
    width: 100%;
    height: 100%;
  }

  .grid-line {
    stroke: var(--border);
    stroke-width: 0.2;
    vector-effect: non-scaling-stroke;
  }

  .y-labels {
    position: absolute;
    right: 16px;
    top: 12px;
    bottom: 12px;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    font-size: 10px;
    color: var(--text-muted);
    pointer-events: none;
  }

  .legend {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    margin-top: 8px;
    font-size: 12px;
    color: var(--text-secondary);
  }

  .legend-item {
    display: flex;
    align-items: center;
    gap: 4px;
  }

  .legend-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    display: inline-block;
  }
</style>
