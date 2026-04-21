<script lang="ts">
  import { untrack } from 'svelte'
  import { t } from '@/lib/i18n/index'
  import { logsStore } from './store.svelte'
  import type { LogEntry } from './types'

  const ROW_HEIGHT = 24
  const OVERSCAN = 10

  let container = $state<HTMLDivElement | null>(null)
  let scrollTop = $state(0)
  let viewportHeight = $state(0)
  let pinnedBottom = $state(true)

  const rows = $derived(logsStore.filtered)
  const totalHeight = $derived(rows.length * ROW_HEIGHT)

  const startIdx = $derived(Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN))
  const visibleCount = $derived(Math.ceil(viewportHeight / ROW_HEIGHT) + OVERSCAN * 2)
  const endIdx = $derived(Math.min(rows.length, startIdx + visibleCount))
  const slice = $derived(rows.slice(startIdx, endIdx))
  const offsetY = $derived(startIdx * ROW_HEIGHT)

  function onScroll(e: Event): void {
    const el = e.currentTarget as HTMLDivElement
    scrollTop = el.scrollTop
    const atBottom = el.scrollTop + el.clientHeight >= el.scrollHeight - 4
    pinnedBottom = atBottom
  }

  function measure(el: HTMLDivElement) {
    container = el
    viewportHeight = el.clientHeight
    const ro = new ResizeObserver(() => { viewportHeight = el.clientHeight })
    ro.observe(el)
    return { destroy: () => ro.disconnect() }
  }

  // Auto-scroll on new entries when user has pinned to bottom.
  $effect(() => {
    const _len = rows.length
    void _len
    untrack(() => {
      if (!container) return
      if (logsStore.autoScroll && pinnedBottom) {
        requestAnimationFrame(() => {
          if (!container) return
          container.scrollTop = container.scrollHeight
        })
      }
    })
  })

  function fmtTime(ms: number): string {
    const d = new Date(ms)
    const pad = (n: number) => String(n).padStart(2, '0')
    return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  }

  function rowClass(entry: LogEntry): string {
    const parts = ['row', `lvl-${entry.level}`]
    if (entry.kind !== 'log') parts.push('conn')
    if (logsStore.selectedId === entry.id) parts.push('selected')
    return parts.join(' ')
  }
</script>

<div class="list" use:measure onscroll={onScroll}>
  {#if rows.length === 0}
    <p class="empty">
      {logsStore.entries.length === 0 ? t('logs.waiting') : t('logs.noMatch')}
    </p>
  {:else}
    <div class="spacer" style="height: {totalHeight}px;">
      <div class="viewport" style="transform: translateY({offsetY}px);">
        {#each slice as entry (entry.id)}
          <button
            type="button"
            class={rowClass(entry)}
            style="height: {ROW_HEIGHT}px;"
            onclick={() => logsStore.select(entry.id)}
          >
            <span class="time">{fmtTime(entry.time)}</span>
            <span class="level">{entry.level}</span>
            {#if entry.kind === 'conn-open'}
              <span class="arrow open">▶</span>
            {:else if entry.kind === 'conn-close'}
              <span class="arrow close">◀</span>
            {/if}
            <span class="msg">{entry.msg}</span>
          </button>
        {/each}
      </div>
    </div>
  {/if}
</div>

<style>
  .list {
    position: relative;
    overflow-y: auto;
    background: var(--shuttle-bg-base);
    font-family: var(--shuttle-font-mono);
    font-size: var(--shuttle-text-xs);
    line-height: 1;
  }
  .spacer { position: relative; width: 100%; }
  .viewport { position: absolute; top: 0; left: 0; right: 0; }
  .empty {
    padding: var(--shuttle-space-6);
    text-align: center;
    color: var(--shuttle-fg-muted);
    font-size: var(--shuttle-text-sm);
  }
  .row {
    display: flex;
    align-items: center;
    gap: var(--shuttle-space-3);
    width: 100%;
    padding: 0 var(--shuttle-space-3);
    background: transparent;
    border: none;
    border-left: 2px solid transparent;
    color: var(--shuttle-fg-primary);
    text-align: left;
    cursor: pointer;
    font-family: inherit;
    font-size: inherit;
  }
  .row:hover { background: var(--shuttle-bg-subtle); }
  .row.selected { background: var(--shuttle-bg-surface); border-left-color: var(--shuttle-accent); }
  .row.conn { border-left-color: var(--shuttle-success); }
  .row.conn.selected { border-left-color: var(--shuttle-accent); }

  .time { color: var(--shuttle-fg-muted); min-width: 68px; }
  .level {
    min-width: 48px;
    color: var(--shuttle-fg-muted);
    text-transform: uppercase;
    font-size: 10px;
    letter-spacing: 0.04em;
  }
  .row.lvl-warn  .level { color: var(--shuttle-warning); }
  .row.lvl-error .level { color: var(--shuttle-danger); }
  .row.lvl-error { color: var(--shuttle-fg-primary); }

  .arrow { font-size: 9px; }
  .arrow.open  { color: var(--shuttle-success); }
  .arrow.close { color: var(--shuttle-fg-muted); }

  .msg {
    flex: 1;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
</style>
