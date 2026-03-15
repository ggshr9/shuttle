<script lang="ts">
  import { connectWS } from '../lib/ws'
  import { api } from '../lib/api'
  import { onMount } from 'svelte'
  import { t } from '../lib/i18n/index'

  let allLogs = $state([])  // Full unfiltered log store
  let connections = $state({}) // Map of connID -> connection details
  let autoScroll = $state(true)
  let showConnections = $state(true)
  let expandedId = $state(null)
  let container

  // Filter state
  let levelFilters = $state({ debug: true, info: true, warn: true, error: true })
  let searchText = $state('')
  let protocolFilter = $state('all')  // all | tcp | udp
  let actionFilter = $state('all')    // all | proxy | direct

  // Reactive filtered logs
  let filteredLogs = $derived.by(() => {
    const search = searchText.toLowerCase()
    return allLogs.filter(log => {
      // Level filter
      if (!levelFilters[log.level]) return false
      // Text search
      if (search && !log.msg.toLowerCase().includes(search)) return false
      // Connection-specific filters (only apply to connection entries)
      if (log.type === 'connection' && log.details) {
        if (protocolFilter !== 'all' && log.details.protocol?.toLowerCase() !== protocolFilter) return false
        if (actionFilter !== 'all' && log.details.rule?.toLowerCase() !== actionFilter) return false
      }
      return true
    })
  })

  onMount(() => {
    // Subscribe to log events
    const logWs = connectWS('/api/logs', (ev) => {
      allLogs = [...allLogs.slice(-499), {
        id: crypto.randomUUID(),
        time: new Date(ev.timestamp).toLocaleTimeString(),
        level: ev.level || 'info',
        msg: ev.message || '',
        type: 'log'
      }]
      scrollToBottom()
    })

    // Subscribe to connection events
    const connWs = connectWS('/api/connections', (ev) => {
      if (ev.conn_state === 'opened') {
        connections[ev.conn_id] = {
          id: ev.conn_id,
          target: ev.target,
          rule: ev.rule || 'default',
          protocol: ev.protocol || 'tcp',
          process: ev.process_name || '',
          startTime: new Date(ev.timestamp),
          state: 'open'
        }
        connections = { ...connections }

        if (showConnections) {
          allLogs = [...allLogs.slice(-499), {
            id: `conn-${ev.conn_id}-open`,
            connId: ev.conn_id,
            time: new Date(ev.timestamp).toLocaleTimeString(),
            level: 'info',
            msg: `Connection opened: ${ev.target}`,
            type: 'connection',
            details: connections[ev.conn_id]
          }]
          scrollToBottom()
        }
      } else if (ev.conn_state === 'closed') {
        const conn = connections[ev.conn_id]
        const details = {
          id: ev.conn_id,
          target: ev.target,
          rule: ev.rule || conn?.rule || 'default',
          protocol: ev.protocol || conn?.protocol || 'tcp',
          process: ev.process_name || conn?.process || '',
          bytesIn: ev.bytes_in || 0,
          bytesOut: ev.bytes_out || 0,
          duration: ev.duration_ms || 0,
          state: 'closed'
        }

        delete connections[ev.conn_id]
        connections = { ...connections }

        if (showConnections) {
          allLogs = [...allLogs.slice(-499), {
            id: `conn-${ev.conn_id}-close`,
            connId: ev.conn_id,
            time: new Date(ev.timestamp).toLocaleTimeString(),
            level: 'info',
            msg: `Connection closed: ${ev.target}`,
            type: 'connection',
            details
          }]
          scrollToBottom()
        }
      }
    })

    return () => {
      logWs.close()
      connWs.close()
    }
  })

  function scrollToBottom() {
    if (autoScroll && container) {
      requestAnimationFrame(() => {
        container.scrollTop = container.scrollHeight
      })
    }
  }

  function clear() {
    allLogs = []
  }

  function toggleLevel(level: string) {
    levelFilters[level] = !levelFilters[level]
    levelFilters = { ...levelFilters }
  }

  function exportLogs() {
    const content = filteredLogs.map(l => {
      let line = `[${l.time}] [${l.level.toUpperCase()}] ${l.msg}`
      if (l.details) {
        line += `\n  ${t('logs.targetColon')} ${l.details.target}`
        line += `\n  ${t('logs.protocolColon')} ${l.details.protocol}`
        if (l.details.rule) line += `\n  ${t('logs.ruleColon')} ${l.details.rule}`
        if (l.details.process) line += `\n  ${t('logs.processColon')} ${l.details.process}`
        if (l.details.duration) line += `\n  ${t('logs.durationColon')} ${formatDuration(l.details.duration)}`
        if (l.details.bytesIn || l.details.bytesOut) {
          line += `\n  ${t('logs.trafficColon')} ${formatBytes(l.details.bytesIn)} in / ${formatBytes(l.details.bytesOut)} out`
        }
      }
      return line
    }).join('\n')
    const blob = new Blob([content], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `shuttle-logs-${new Date().toISOString().slice(0, 10)}.txt`
    a.click()
    URL.revokeObjectURL(url)
  }

  function toggleExpand(id) {
    expandedId = expandedId === id ? null : id
  }

  function formatBytes(bytes) {
    if (!bytes) return '0 B'
    if (bytes < 1024) return bytes + ' B'
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
    if (bytes < 1024 * 1024 * 1024) return (bytes / 1024 / 1024).toFixed(2) + ' MB'
    return (bytes / 1024 / 1024 / 1024).toFixed(2) + ' GB'
  }

  function formatDuration(ms) {
    if (!ms) return '0ms'
    if (ms < 1000) return ms + 'ms'
    if (ms < 60000) return (ms / 1000).toFixed(1) + 's'
    return (ms / 60000).toFixed(1) + 'm'
  }

  function getActiveCount() {
    return Object.keys(connections).length
  }
</script>

<div class="page">
  <div class="header">
    <h2>{t('logs.title')}</h2>
    <div class="status-badge" class:active={getActiveCount() > 0}>
      {t('logs.activeConnections', { count: getActiveCount() })}
    </div>
    <div class="controls">
      <label>
        <input type="checkbox" bind:checked={showConnections} /> {t('logs.showConnections')}
      </label>
      <label>
        <input type="checkbox" bind:checked={autoScroll} /> {t('logs.autoScroll')}
      </label>
      <button onclick={clear}>{t('logs.clear')}</button>
      <button onclick={exportLogs} disabled={allLogs.length === 0}>{t('logs.export')}</button>
    </div>
  </div>

  <div class="filter-bar">
    <div class="filter-group">
      <span class="filter-label">{t('logs.level')}:</span>
      <button class="level-toggle" class:active={levelFilters.debug} onclick={() => toggleLevel('debug')}>{t('logs.debug')}</button>
      <button class="level-toggle" class:active={levelFilters.info} onclick={() => toggleLevel('info')}>{t('logs.info')}</button>
      <button class="level-toggle warn" class:active={levelFilters.warn} onclick={() => toggleLevel('warn')}>{t('logs.warn')}</button>
      <button class="level-toggle error" class:active={levelFilters.error} onclick={() => toggleLevel('error')}>{t('logs.error')}</button>
    </div>
    <div class="filter-group">
      <input
        type="text"
        class="search-input"
        placeholder={t('logs.searchPlaceholder')}
        bind:value={searchText}
      />
    </div>
    {#if showConnections}
      <div class="filter-group">
        <span class="filter-label">{t('logs.protocol')}:</span>
        <select class="filter-select" bind:value={protocolFilter}>
          <option value="all">{t('logs.filterAll')}</option>
          <option value="tcp">TCP</option>
          <option value="udp">UDP</option>
        </select>
      </div>
      <div class="filter-group">
        <span class="filter-label">{t('logs.action')}:</span>
        <select class="filter-select" bind:value={actionFilter}>
          <option value="all">{t('logs.filterAll')}</option>
          <option value="proxy">{t('routing.proxy')}</option>
          <option value="direct">{t('routing.direct')}</option>
        </select>
      </div>
    {/if}
    {#if filteredLogs.length !== allLogs.length}
      <span class="filter-count">{t('logs.showing', { shown: filteredLogs.length, total: allLogs.length })}</span>
    {/if}
  </div>

  <div class="log-container" bind:this={container}>
    {#each filteredLogs as log (log.id)}
      <div
        class="line"
        class:connection={log.type === 'connection'}
        class:expanded={expandedId === log.id}
        class:clickable={log.details}
      >
        <div class="line-main" onclick={() => log.details && toggleExpand(log.id)}>
          <span class="time">{log.time}</span>
          <span class="level" class:warn={log.level === 'warn'} class:error={log.level === 'error'}>
            {log.level}
          </span>
          {#if log.type === 'connection'}
            <span class="conn-icon">{log.details?.state === 'open' ? '→' : '←'}</span>
          {/if}
          <span class="msg">{log.msg}</span>
          {#if log.details}
            <span class="expand-icon">{expandedId === log.id ? '▼' : '▶'}</span>
          {/if}
        </div>

        {#if expandedId === log.id && log.details}
          <div class="details">
            <div class="detail-row">
              <span class="detail-label">{t('logs.targetColon')}</span>
              <span class="detail-value">{log.details.target}</span>
            </div>
            <div class="detail-row">
              <span class="detail-label">{t('logs.protocolColon')}</span>
              <span class="detail-value protocol">{log.details.protocol.toUpperCase()}</span>
            </div>
            {#if log.details.rule}
              <div class="detail-row">
                <span class="detail-label">{t('logs.ruleColon')}</span>
                <span class="detail-value rule">{log.details.rule}</span>
              </div>
            {/if}
            {#if log.details.process}
              <div class="detail-row">
                <span class="detail-label">{t('logs.processColon')}</span>
                <span class="detail-value">{log.details.process}</span>
              </div>
            {/if}
            {#if log.details.state === 'closed'}
              <div class="detail-row">
                <span class="detail-label">{t('logs.durationColon')}</span>
                <span class="detail-value">{formatDuration(log.details.duration)}</span>
              </div>
              <div class="detail-row">
                <span class="detail-label">{t('logs.trafficColon')}</span>
                <span class="detail-value">
                  <span class="traffic-in">↓ {formatBytes(log.details.bytesIn)}</span>
                  <span class="traffic-out">↑ {formatBytes(log.details.bytesOut)}</span>
                </span>
              </div>
            {/if}
          </div>
        {/if}
      </div>
    {/each}
    {#if allLogs.length === 0}
      <p class="empty">{t('logs.waiting')}</p>
    {:else if filteredLogs.length === 0}
      <p class="empty">{t('logs.noMatch')}</p>
    {/if}
  </div>
</div>

<style>
  .page { height: calc(100vh - 120px); display: flex; flex-direction: column; }

  .header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 12px;
    gap: 12px;
  }

  h2 { font-size: 18px; margin: 0; white-space: nowrap; }

  .status-badge {
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 4px 10px;
    font-size: 11px;
    color: var(--text-secondary);
  }

  .status-badge.active {
    background: rgba(63, 185, 80, 0.1);
    border-color: var(--accent-green);
    color: var(--accent-green);
  }

  .controls {
    display: flex;
    gap: 12px;
    align-items: center;
    font-size: 13px;
    color: var(--text-secondary);
    margin-left: auto;
  }

  .controls button {
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
    color: var(--text-primary);
    border-radius: 6px;
    padding: 4px 12px;
    cursor: pointer;
    font-size: 12px;
  }

  .controls button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .filter-bar {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 8px;
    flex-wrap: wrap;
  }

  .filter-group {
    display: flex;
    align-items: center;
    gap: 4px;
  }

  .filter-label {
    font-size: 11px;
    color: var(--text-muted);
    white-space: nowrap;
  }

  .level-toggle {
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
    color: var(--text-secondary);
    border-radius: 4px;
    padding: 2px 8px;
    cursor: pointer;
    font-size: 11px;
    opacity: 0.5;
    transition: opacity 0.15s, background 0.15s;
  }

  .level-toggle.active {
    opacity: 1;
    background: rgba(88, 166, 255, 0.15);
    border-color: var(--accent);
    color: var(--accent);
  }

  .level-toggle.warn.active {
    background: rgba(210, 153, 34, 0.15);
    border-color: #d29922;
    color: #d29922;
  }

  .level-toggle.error.active {
    background: rgba(248, 81, 73, 0.15);
    border-color: var(--accent-red);
    color: var(--accent-red);
  }

  .search-input {
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
    color: var(--text-primary);
    border-radius: 6px;
    padding: 3px 8px;
    font-size: 12px;
    width: 180px;
    outline: none;
  }

  .search-input:focus {
    border-color: var(--accent);
  }

  .search-input::placeholder {
    color: var(--text-muted);
  }

  .filter-select {
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
    color: var(--text-primary);
    border-radius: 6px;
    padding: 3px 6px;
    font-size: 11px;
    outline: none;
    cursor: pointer;
  }

  .filter-select:focus {
    border-color: var(--accent);
  }

  .filter-count {
    font-size: 11px;
    color: var(--text-muted);
    margin-left: auto;
    white-space: nowrap;
  }

  .log-container {
    flex: 1;
    overflow-y: auto;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 8px;
    font-family: 'Cascadia Code', 'Fira Code', monospace;
    font-size: 12px;
  }

  .line {
    padding: 2px 0;
    line-height: 1.5;
    border-radius: 4px;
  }

  .line.clickable {
    cursor: pointer;
  }

  .line.clickable:hover {
    background: rgba(88, 166, 255, 0.05);
  }

  .line.expanded {
    background: rgba(88, 166, 255, 0.08);
  }

  .line.connection {
    border-left: 2px solid var(--accent-green);
    padding-left: 8px;
    margin-left: -8px;
  }

  .line-main {
    display: flex;
    gap: 8px;
    align-items: center;
  }

  .time { color: var(--text-muted); min-width: 70px; }
  .level { color: var(--accent); min-width: 40px; text-transform: uppercase; }
  .level.warn { color: #d29922; }
  .level.error { color: var(--accent-red); }
  .msg { color: #c9d1d9; flex: 1; }
  .empty { color: var(--text-muted); text-align: center; margin-top: 40px; }

  .conn-icon {
    color: var(--accent-green);
    font-size: 10px;
  }

  .expand-icon {
    color: var(--text-muted);
    font-size: 10px;
    margin-left: auto;
    padding-right: 8px;
  }

  .details {
    margin-top: 8px;
    margin-left: 78px;
    padding: 8px 12px;
    background: rgba(13, 17, 23, 0.8);
    border: 1px solid var(--bg-tertiary);
    border-radius: 6px;
    margin-bottom: 4px;
  }

  .detail-row {
    display: flex;
    gap: 8px;
    padding: 2px 0;
  }

  .detail-label {
    color: #6e7681;
    min-width: 70px;
  }

  .detail-value {
    color: #c9d1d9;
  }

  .detail-value.protocol {
    color: var(--accent);
    background: rgba(88, 166, 255, 0.1);
    padding: 1px 6px;
    border-radius: 3px;
    font-size: 10px;
  }

  .detail-value.rule {
    color: #d29922;
  }

  .traffic-in {
    color: var(--accent-green);
    margin-right: 12px;
  }

  .traffic-out {
    color: var(--accent);
  }
</style>
