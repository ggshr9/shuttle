<script>
  import { connectWS } from '../lib/ws.js'
  import { api } from '../lib/api.js'
  import { onMount } from 'svelte'

  let logs = $state([])
  let connections = $state({}) // Map of connID -> connection details
  let autoScroll = $state(true)
  let showConnections = $state(true)
  let expandedId = $state(null)
  let container

  onMount(() => {
    // Subscribe to log events
    const logWs = connectWS('/api/logs', (ev) => {
      logs = [...logs.slice(-499), {
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
          logs = [...logs.slice(-499), {
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
          logs = [...logs.slice(-499), {
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
    logs = []
  }

  function exportLogs() {
    const content = logs.map(l => {
      let line = `[${l.time}] [${l.level.toUpperCase()}] ${l.msg}`
      if (l.details) {
        line += `\n  Target: ${l.details.target}`
        line += `\n  Protocol: ${l.details.protocol}`
        if (l.details.rule) line += `\n  Rule: ${l.details.rule}`
        if (l.details.process) line += `\n  Process: ${l.details.process}`
        if (l.details.duration) line += `\n  Duration: ${formatDuration(l.details.duration)}`
        if (l.details.bytesIn || l.details.bytesOut) {
          line += `\n  Traffic: ${formatBytes(l.details.bytesIn)} in / ${formatBytes(l.details.bytesOut)} out`
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
    <h2>Logs</h2>
    <div class="status-badge" class:active={getActiveCount() > 0}>
      {getActiveCount()} active connections
    </div>
    <div class="controls">
      <label>
        <input type="checkbox" bind:checked={showConnections} /> Show Connections
      </label>
      <label>
        <input type="checkbox" bind:checked={autoScroll} /> Auto-scroll
      </label>
      <button onclick={clear}>Clear</button>
      <button onclick={exportLogs} disabled={logs.length === 0}>Export</button>
    </div>
  </div>

  <div class="log-container" bind:this={container}>
    {#each logs as log (log.id)}
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
              <span class="detail-label">Target:</span>
              <span class="detail-value">{log.details.target}</span>
            </div>
            <div class="detail-row">
              <span class="detail-label">Protocol:</span>
              <span class="detail-value protocol">{log.details.protocol.toUpperCase()}</span>
            </div>
            {#if log.details.rule}
              <div class="detail-row">
                <span class="detail-label">Rule:</span>
                <span class="detail-value rule">{log.details.rule}</span>
              </div>
            {/if}
            {#if log.details.process}
              <div class="detail-row">
                <span class="detail-label">Process:</span>
                <span class="detail-value">{log.details.process}</span>
              </div>
            {/if}
            {#if log.details.state === 'closed'}
              <div class="detail-row">
                <span class="detail-label">Duration:</span>
                <span class="detail-value">{formatDuration(log.details.duration)}</span>
              </div>
              <div class="detail-row">
                <span class="detail-label">Traffic:</span>
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
    {#if logs.length === 0}
      <p class="empty">Waiting for log events...</p>
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

  h2 { font-size: 18px; margin: 0; }

  .status-badge {
    background: #21262d;
    border: 1px solid #2d333b;
    border-radius: 12px;
    padding: 4px 10px;
    font-size: 11px;
    color: #8b949e;
  }

  .status-badge.active {
    background: rgba(63, 185, 80, 0.1);
    border-color: #3fb950;
    color: #3fb950;
  }

  .controls {
    display: flex;
    gap: 12px;
    align-items: center;
    font-size: 13px;
    color: #8b949e;
    margin-left: auto;
  }

  .controls button {
    background: #21262d;
    border: 1px solid #2d333b;
    color: #e1e4e8;
    border-radius: 6px;
    padding: 4px 12px;
    cursor: pointer;
    font-size: 12px;
  }

  .controls button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .log-container {
    flex: 1;
    overflow-y: auto;
    background: #0d1117;
    border: 1px solid #2d333b;
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
    border-left: 2px solid #3fb950;
    padding-left: 8px;
    margin-left: -8px;
  }

  .line-main {
    display: flex;
    gap: 8px;
    align-items: center;
  }

  .time { color: #484f58; min-width: 70px; }
  .level { color: #58a6ff; min-width: 40px; text-transform: uppercase; }
  .level.warn { color: #d29922; }
  .level.error { color: #f85149; }
  .msg { color: #c9d1d9; flex: 1; }
  .empty { color: #484f58; text-align: center; margin-top: 40px; }

  .conn-icon {
    color: #3fb950;
    font-size: 10px;
  }

  .expand-icon {
    color: #484f58;
    font-size: 10px;
    margin-left: auto;
    padding-right: 8px;
  }

  .details {
    margin-top: 8px;
    margin-left: 78px;
    padding: 8px 12px;
    background: rgba(13, 17, 23, 0.8);
    border: 1px solid #21262d;
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
    color: #58a6ff;
    background: rgba(88, 166, 255, 0.1);
    padding: 1px 6px;
    border-radius: 3px;
    font-size: 10px;
  }

  .detail-value.rule {
    color: #d29922;
  }

  .traffic-in {
    color: #3fb950;
    margin-right: 12px;
  }

  .traffic-out {
    color: #58a6ff;
  }
</style>
