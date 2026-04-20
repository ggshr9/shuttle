<script lang="ts">
  import { AsyncBoundary, Button, Icon, Section } from '@/ui'
  import { useServers, removeServer, removeMany, autoSelect, runSpeedtest } from './resource.svelte'
  import ServerTable from './ServerTable.svelte'
  import AddServerDialog from './AddServerDialog.svelte'
  import ImportDialog from './ImportDialog.svelte'
  import DeleteConfirm from './DeleteConfirm.svelte'

  const res = useServers()

  let selected = $state<Set<string>>(new Set())
  let addOpen = $state(false)
  let importOpen = $state(false)
  let deleteOpen = $state(false)
  let pendingDelete = $state<string[]>([])

  function openSingleDelete(addr: string) {
    pendingDelete = [addr]
    deleteOpen = true
  }

  function openBatchDelete() {
    pendingDelete = Array.from(selected)
    deleteOpen = true
  }

  async function confirmDelete() {
    if (pendingDelete.length === 1) {
      await removeServer(pendingDelete[0])
    } else {
      await removeMany(pendingDelete)
    }
    selected = new Set()
  }

  async function testSelected() {
    await runSpeedtest(Array.from(selected))
  }

  async function testAll() {
    if (!res.data) return
    await runSpeedtest(res.data.servers.map((s) => s.addr))
  }
</script>

<Section
  title="Servers"
  description={res.data ? `${res.data.servers.length} configured` : undefined}
>
  {#snippet actions()}
    <Button variant="ghost" onclick={testAll}>Test all</Button>
    <Button variant="ghost" onclick={() => autoSelect()}>
      <Icon name="check" size={14} /> Auto-select
    </Button>
    <Button variant="ghost" onclick={() => (importOpen = true)}>
      Import
    </Button>
    <Button variant="primary" onclick={() => (addOpen = true)}>
      <Icon name="plus" size={14} /> Add server
    </Button>
  {/snippet}

  {#if selected.size > 0}
    <div class="sel-bar">
      <span class="count">{selected.size} selected</span>
      <Button size="sm" variant="secondary" onclick={testSelected}>Speed test</Button>
      <Button size="sm" variant="danger"    onclick={openBatchDelete}>Delete</Button>
      <Button size="sm" variant="ghost"     onclick={() => (selected = new Set())}>Cancel</Button>
    </div>
  {/if}

  <AsyncBoundary resource={res}>
    {#snippet children(list)}
      <ServerTable
        servers={list.servers}
        activeAddr={list.active.addr}
        {selected}
        onSelectedChange={(s) => (selected = s)}
        onDelete={openSingleDelete}
      />
    {/snippet}
  </AsyncBoundary>
</Section>

<AddServerDialog bind:open={addOpen} />
<ImportDialog bind:open={importOpen} />
<DeleteConfirm
  bind:open={deleteOpen}
  count={pendingDelete.length}
  onConfirm={confirmDelete}
/>

<style>
  .sel-bar {
    display: flex; align-items: center; gap: var(--shuttle-space-2);
    padding: var(--shuttle-space-2) var(--shuttle-space-3);
    margin-bottom: var(--shuttle-space-2);
    background: var(--shuttle-bg-subtle);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    font-size: var(--shuttle-text-sm);
  }
  .count {
    margin-right: auto;
    color: var(--shuttle-fg-primary);
    font-weight: var(--shuttle-weight-medium);
  }
</style>
