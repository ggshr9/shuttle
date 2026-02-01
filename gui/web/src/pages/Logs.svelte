<script>
  import { connectWS } from '../lib/ws.js'
  import { onMount } from 'svelte'

  let logs = $state([])
  let autoScroll = $state(true)
  let container

  onMount(() => {
    const ws = connectWS('/api/logs', (ev) => {
      logs = [...logs.slice(-499), {
        time: new Date(ev.timestamp).toLocaleTimeString(),
        level: ev.level || 'info',
        msg: ev.message || '',
      }]
      if (autoScroll && container) {
        requestAnimationFrame(() => {
          container.scrollTop = container.scrollHeight
        })
      }
    })
    return () => ws.close()
  })

  function clear() {
    logs = []
  }
</script>

<div class="page">
  <div class="header">
    <h2>Logs</h2>
    <div class="controls">
      <label>
        <input type="checkbox" bind:checked={autoScroll} /> Auto-scroll
      </label>
      <button onclick={clear}>Clear</button>
    </div>
  </div>

  <div class="log-container" bind:this={container}>
    {#each logs as log}
      <div class="line">
        <span class="time">{log.time}</span>
        <span class="level" class:warn={log.level === 'warn'} class:error={log.level === 'error'}>
          {log.level}
        </span>
        <span class="msg">{log.msg}</span>
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
  }

  h2 { font-size: 18px; margin: 0; }

  .controls {
    display: flex;
    gap: 12px;
    align-items: center;
    font-size: 13px;
    color: #8b949e;
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
    display: flex;
    gap: 8px;
    padding: 2px 0;
    line-height: 1.5;
  }

  .time { color: #484f58; min-width: 70px; }
  .level { color: #58a6ff; min-width: 40px; text-transform: uppercase; }
  .level.warn { color: #d29922; }
  .level.error { color: #f85149; }
  .msg { color: #c9d1d9; }
  .empty { color: #484f58; text-align: center; margin-top: 40px; }
</style>
