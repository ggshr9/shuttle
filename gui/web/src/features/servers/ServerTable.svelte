<script lang="ts">
  import { Empty, Card } from '@/ui'
  import ServerRow from './ServerRow.svelte'
  import ServerRowExpanded from './ServerRowExpanded.svelte'
  import type { Server } from '@/lib/api/types'

  interface Props {
    servers: Server[]
    activeAddr: string
    selected: Set<string>
    onSelectedChange: (next: Set<string>) => void
    onDelete: (addr: string) => void
  }

  let { servers, activeAddr, selected, onSelectedChange, onDelete }: Props = $props()

  const expanded = $state<Set<string>>(new Set())

  function toggleSelect(addr: string, v: boolean) {
    const next = new Set(selected)
    if (v) next.add(addr)
    else next.delete(addr)
    onSelectedChange(next)
  }

  function toggleExpanded(addr: string) {
    if (expanded.has(addr)) expanded.delete(addr)
    else expanded.add(addr)
  }

  function toggleAll(v: boolean) {
    onSelectedChange(v ? new Set(servers.map((s) => s.addr)) : new Set())
  }

  const allSelected = $derived(servers.length > 0 && selected.size === servers.length)
  const someSelected = $derived(selected.size > 0 && !allSelected)
</script>

{#if servers.length === 0}
  <Card>
    <Empty
      icon="servers"
      title="No servers"
      description="Add one or import a subscription to get started."
    />
  </Card>
{:else}
  <div class="table">
    <div class="header">
      <span class="check">
        <input
          type="checkbox"
          checked={allSelected}
          indeterminate={someSelected}
          onchange={(e) => toggleAll((e.target as HTMLInputElement).checked)}
          aria-label="Select all"
        />
      </span>
      <span></span>
      <span>Name</span>
      <span>Address</span>
      <span class="lat">Latency</span>
      <span>Protocol</span>
      <span></span>
    </div>

    {#each servers as s (s.addr)}
      <ServerRow
        server={s}
        isActive={s.addr === activeAddr}
        selected={selected.has(s.addr)}
        expanded={expanded.has(s.addr)}
        onSelectedChange={(v) => toggleSelect(s.addr, v)}
        onExpandedToggle={() => toggleExpanded(s.addr)}
        onDelete={() => onDelete(s.addr)}
      />
      {#if expanded.has(s.addr)}
        <ServerRowExpanded server={s} isActive={s.addr === activeAddr} />
      {/if}
    {/each}
  </div>
{/if}

<style>
  .table {
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    background: var(--shuttle-bg-surface);
    overflow: hidden;
  }
  .header {
    display: grid;
    grid-template-columns: 32px 16px 2fr 3fr 80px 80px auto;
    align-items: center;
    gap: var(--shuttle-space-3);
    height: 36px;
    padding: 0 var(--shuttle-space-4);
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-muted);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    background: var(--shuttle-bg-subtle);
    border-bottom: 1px solid var(--shuttle-border);
  }
  .header .lat { text-align: right; }
  .check input { cursor: pointer; }
</style>
