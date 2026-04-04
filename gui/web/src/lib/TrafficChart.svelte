<script lang="ts">
  import { api, type PeriodStats, type StatsHistory } from './api'

  interface DailyHistoryEntry {
    date: string
    bytes_sent: number
    bytes_received: number
    connections: number
  }

  interface StatsHistoryResponse {
    history: DailyHistoryEntry[]
    total: StatsHistory
  }
  import { onMount } from 'svelte'
  import { t } from './i18n/index'

  type ViewMode = 'daily' | 'weekly' | 'monthly'

  let mode: ViewMode = $state('daily')
  let dailyData: PeriodStats[] = $state([])
  let weeklyData: PeriodStats[] = $state([])
  let monthlyData: PeriodStats[] = $state([])
  let loading = $state(false)

  let data = $derived(
    mode === 'daily' ? dailyData :
    mode === 'weekly' ? weeklyData :
    monthlyData
  )

  let totalSent = $derived(data.reduce((s, d) => s + d.bytes_sent, 0))
  let totalRecv = $derived(data.reduce((s, d) => s + d.bytes_recv, 0))
  let maxBytes = $derived(Math.max(...data.map(d => d.bytes_sent + d.bytes_recv), 1))

  onMount(() => {
    loadAll()
  })

  async function loadAll() {
    loading = true
    try {
      const [historyRes, weeklyRes, monthlyRes] = await Promise.all([
        api.getStatsHistory(7) as Promise<StatsHistoryResponse>,
        api.getWeeklyStats(4),
        api.getMonthlyStats(6),
      ])
      // Convert daily history to PeriodStats format
      const history: DailyHistoryEntry[] = historyRes.history || []
      dailyData = history.map((d) => ({
        period: d.date,
        bytes_sent: d.bytes_sent || 0,
        bytes_recv: d.bytes_received || 0,
        connections: d.connections || 0,
        days: 1,
      }))
      weeklyData = weeklyRes || []
      monthlyData = monthlyRes || []
    } catch {
      // Stats not available
    } finally {
      loading = false
    }
  }

  function formatBytes(bytes: number): string {
    if (bytes < 1024) return bytes + ' B'
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
    if (bytes < 1024 * 1024 * 1024) return (bytes / 1024 / 1024).toFixed(1) + ' MB'
    return (bytes / 1024 / 1024 / 1024).toFixed(2) + ' GB'
  }

  function periodLabel(period: string): string {
    if (mode === 'daily') {
      // period is "YYYY-MM-DD"
      const d = new Date(period)
      const days = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
      return days[d.getDay()]
    }
    if (mode === 'weekly') {
      // period is "YYYY-Wxx"
      return period.slice(5) // "W11"
    }
    // monthly: "YYYY-MM"
    const months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
    const m = parseInt(period.slice(5), 10)
    return months[m - 1] || period.slice(5)
  }

  function setMode(m: ViewMode) {
    mode = m
  }
</script>

<div class="traffic-chart-wrapper">
  <div class="traffic-header">
    <h3>{t('traffic.title')}</h3>
    <div class="traffic-total">
      {t('traffic.total')}: {formatBytes(totalSent + totalRecv)}
    </div>
  </div>

  <div class="mode-tabs">
    <button class:active={mode === 'daily'} onclick={() => setMode('daily')}>{t('traffic.daily')}</button>
    <button class:active={mode === 'weekly'} onclick={() => setMode('weekly')}>{t('traffic.weekly')}</button>
    <button class:active={mode === 'monthly'} onclick={() => setMode('monthly')}>{t('traffic.monthly')}</button>
  </div>

  {#if loading}
    <div class="traffic-loading">{t('common.loading')}</div>
  {:else if data.length === 0}
    <div class="traffic-empty">{t('traffic.noData')}</div>
  {:else}
    <div class="traffic-bars">
      {#each data as item}
        {@const total = item.bytes_sent + item.bytes_recv}
        {@const pct = Math.max((total / maxBytes) * 100, 2)}
        {@const sentPct = total > 0 ? (item.bytes_sent / total) * 100 : 50}
        <div class="bar-col">
          <div class="bar-stack" style="height: {pct}%">
            <div class="bar-sent" style="height: {sentPct}%" title="{t('traffic.sent')}: {formatBytes(item.bytes_sent)}"></div>
            <div class="bar-recv" style="height: {100 - sentPct}%" title="{t('traffic.received')}: {formatBytes(item.bytes_recv)}"></div>
          </div>
          <span class="bar-label">{periodLabel(item.period)}</span>
          <span class="bar-value">{formatBytes(total)}</span>
        </div>
      {/each}
    </div>
    <div class="traffic-legend">
      <span class="legend-item"><span class="legend-dot sent"></span> {t('traffic.sent')}</span>
      <span class="legend-item"><span class="legend-dot recv"></span> {t('traffic.received')}</span>
    </div>
  {/if}
</div>

<style>
  .traffic-chart-wrapper {
    margin: 20px 0;
  }

  .traffic-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 8px;
  }

  .traffic-header h3 {
    font-size: 14px;
    color: var(--text-secondary);
    margin: 0;
    text-align: left;
  }

  .traffic-total {
    font-size: 13px;
    color: var(--accent-green);
    font-weight: 600;
  }

  .mode-tabs {
    display: flex;
    gap: 4px;
    margin-bottom: 12px;
    background: var(--bg-surface);
    border-radius: var(--radius-sm);
    padding: 3px;
    width: fit-content;
  }

  .mode-tabs button {
    background: none;
    border: none;
    padding: 5px 14px;
    font-size: 12px;
    color: var(--text-secondary);
    cursor: pointer;
    border-radius: 4px;
    font-family: inherit;
    font-weight: 500;
    transition: all 0.2s;
  }

  .mode-tabs button:hover {
    color: var(--text-primary);
  }

  .mode-tabs button.active {
    background: var(--accent);
    color: #fff;
  }

  .traffic-bars {
    display: flex;
    gap: 8px;
    justify-content: center;
    align-items: flex-end;
    height: 140px;
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    padding: 16px;
  }

  .bar-col {
    display: flex;
    flex-direction: column;
    align-items: center;
    flex: 1;
    max-width: 60px;
    height: 100%;
    justify-content: flex-end;
  }

  .bar-stack {
    width: 100%;
    max-width: 36px;
    display: flex;
    flex-direction: column;
    border-radius: 4px 4px 0 0;
    overflow: hidden;
    min-height: 2px;
  }

  .bar-sent {
    background: var(--accent);
  }

  .bar-recv {
    background: var(--accent-green);
  }

  .bar-label {
    font-size: 10px;
    color: var(--text-secondary);
    margin-top: 4px;
  }

  .bar-value {
    font-size: 9px;
    color: var(--text-muted);
  }

  .traffic-legend {
    display: flex;
    justify-content: center;
    gap: 16px;
    margin-top: 8px;
  }

  .legend-item {
    display: flex;
    align-items: center;
    gap: 4px;
    font-size: 11px;
    color: var(--text-secondary);
  }

  .legend-dot {
    width: 10px;
    height: 10px;
    border-radius: 2px;
  }

  .legend-dot.sent {
    background: var(--accent);
  }

  .legend-dot.recv {
    background: var(--accent-green);
  }

  .traffic-loading,
  .traffic-empty {
    text-align: center;
    color: var(--text-secondary);
    font-size: 13px;
    padding: 32px;
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
  }
</style>
