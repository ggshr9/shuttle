<script lang="ts">
  import { Button, Icon, Badge } from '@/ui'
  import { useSpeedtestResult, setActive } from './resource.svelte'
  import type { Server } from '@/lib/api/types'

  interface Props {
    server: Server
    isActive: boolean
    selected: boolean
    expanded: boolean
    onSelectedChange: (v: boolean) => void
    onExpandedToggle: () => void
    onDelete: () => void
  }

  let {
    server, isActive, selected, expanded,
    onSelectedChange, onExpandedToggle, onDelete,
  }: Props = $props()

  const result = $derived(useSpeedtestResult(server.addr))

  function inferProtocol(addr: string): string {
    if (addr.startsWith('ss://'))      return 'ss'
    if (addr.startsWith('vmess://'))   return 'vmess'
    if (addr.startsWith('trojan://'))  return 'trojan'
    if (addr.startsWith('shuttle://')) return 'shuttle'
    if (/^[^/:]+:\d+$/.test(addr))     return 'shuttle'
    return '—'
  }

  const protocol = $derived(inferProtocol(server.addr))
  const statusClass = $derived(
    !result ? 'unknown' : result.available ? 'ok' : 'bad'
  )
</script>

<div class="row" class:active={isActive} class:selected>
  <span class="check">
    <input
      type="checkbox"
      checked={selected}
      onchange={(e) => onSelectedChange((e.target as HTMLInputElement).checked)}
      aria-label={`Select ${server.name || server.addr}`}
    />
  </span>
  <span class={`status ${statusClass}`} aria-label={statusClass}></span>
  <span class="name">{server.name || '—'}</span>
  <span class="addr">{server.addr}</span>
  <span class="lat">
    {#if result}{result.latency} ms{:else}— ms{/if}
  </span>
  <span class="proto">
    <Badge>{protocol}</Badge>
  </span>
  <span class="actions">
    <Button size="sm" variant="ghost" onclick={onExpandedToggle}>
      <Icon name={expanded ? 'chevronDown' : 'chevronRight'} size={14} />
    </Button>
    {#if !isActive}
      <Button size="sm" variant="ghost" onclick={() => setActive(server)}>
        <Icon name="check" size={14} title="Set active" />
      </Button>
    {/if}
    <Button size="sm" variant="ghost" onclick={onDelete}>
      <Icon name="trash" size={14} title="Delete" />
    </Button>
  </span>
</div>

<style>
  .row {
    display: grid;
    grid-template-columns: 32px 16px 2fr 3fr 80px 80px auto;
    align-items: center;
    gap: var(--shuttle-space-3);
    height: 48px;
    padding: 0 var(--shuttle-space-4);
    border-top: 1px solid var(--shuttle-border);
    border-left: 2px solid transparent;
    font-size: var(--shuttle-text-sm);
  }
  .row:first-child { border-top: 0; }
  .row.active   { border-left-color: var(--shuttle-accent); }
  .row.selected { background: var(--shuttle-bg-subtle); }

  .check { display: flex; }
  .check input { cursor: pointer; }

  .status {
    width: 8px; height: 8px; border-radius: 50%;
    background: var(--shuttle-fg-muted);
  }
  .status.ok  { background: var(--shuttle-success); }
  .status.bad { background: var(--shuttle-danger); }

  .name {
    font-weight: var(--shuttle-weight-medium);
    color: var(--shuttle-fg-primary);
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .addr {
    font-family: var(--shuttle-font-mono);
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-secondary);
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .lat {
    font-family: var(--shuttle-font-mono);
    color: var(--shuttle-fg-secondary);
    font-variant-numeric: tabular-nums;
    text-align: right;
  }
  .actions { display: flex; gap: 2px; justify-content: flex-end; }
</style>
