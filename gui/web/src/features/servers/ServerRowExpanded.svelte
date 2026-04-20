<script lang="ts">
  import { Input, Button, StatRow } from '@/ui'
  import { setActive, useSpeedtestResult, runSpeedtest } from './resource.svelte'
  import type { Server } from '@/lib/api/types'

  interface Props {
    server: Server
    isActive: boolean
  }
  let { server, isActive }: Props = $props()

  // Local drafts. The backend currently has no dedicated updateServer endpoint
  // so name/sni edits don't round-trip back until the user invokes
  // "Set as active" (which sends the whole record). We keep the fields visible
  // for familiarity; P5+ may add a direct update endpoint.
  let name = $state(server.name ?? '')
  let sni = $state(server.sni ?? '')

  const result = $derived(useSpeedtestResult(server.addr))

  async function test() {
    await runSpeedtest([server.addr])
  }

  async function makeActive() {
    await setActive({ ...server, name: name || undefined, sni: sni || undefined })
  }
</script>

<div class="pane">
  <div class="fields">
    <Input label="Name" bind:value={name} />
    <Input label="Server address" value={server.addr} disabled />
    <Input label="SNI" bind:value={sni} />
  </div>

  <div class="side">
    {#if result}
      <StatRow label="Latency" value={`${result.latency} ms`} mono />
      <StatRow label="Available" value={result.available ? 'yes' : 'no'} />
    {:else}
      <p class="hint">Not yet tested.</p>
    {/if}
    <div class="actions">
      <Button size="sm" variant="secondary" onclick={test}>Speed test</Button>
      {#if !isActive}
        <Button size="sm" variant="primary" onclick={makeActive}>Set as active</Button>
      {/if}
    </div>
  </div>
</div>

<style>
  .pane {
    display: grid; grid-template-columns: 1fr 240px;
    gap: var(--shuttle-space-5);
    padding: var(--shuttle-space-4) var(--shuttle-space-4) var(--shuttle-space-4) var(--shuttle-space-6);
    background: var(--shuttle-bg-subtle);
    border-top: 1px solid var(--shuttle-border);
  }
  .fields { display: flex; flex-direction: column; gap: var(--shuttle-space-3); }
  .side { display: flex; flex-direction: column; gap: var(--shuttle-space-2); }
  .hint { font-size: var(--shuttle-text-sm); color: var(--shuttle-fg-muted); margin: 0; }
  .actions { margin-top: var(--shuttle-space-3); display: flex; gap: var(--shuttle-space-2); }
</style>
