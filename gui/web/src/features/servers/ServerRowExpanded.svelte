<script lang="ts">
  import { Input, Button, StatRow } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { setActive, useSpeedtestResult, runSpeedtest } from './resource.svelte'
  import type { Server } from '@/lib/api/types'

  interface Props {
    server: Server
    isActive: boolean
  }
  let { server, isActive }: Props = $props()

  // Local drafts seeded from the prop at mount. ServerTable keys this row by
  // server.addr so a different server always remounts — stale-capture doesn't
  // apply here.
  // svelte-ignore state_referenced_locally
  let name = $state(server.name ?? '')
  // svelte-ignore state_referenced_locally
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
    <Input label={t('servers.name')} bind:value={name} />
    <Input label={t('servers.serverAddress')} value={server.addr} disabled />
    <Input label={t('servers.sni')} bind:value={sni} />
  </div>

  <div class="side">
    {#if result}
      <StatRow label={t('servers.columns.latency')} value={`${result.latency} ms`} mono />
      <StatRow label={t('servers.available')} value={result.available ? t('common.yes') : t('common.no')} />
    {:else}
      <p class="hint">{t('servers.notTested')}</p>
    {/if}
    <div class="actions">
      <Button size="sm" variant="secondary" onclick={test}>{t('servers.speedTest')}</Button>
      {#if !isActive}
        <Button size="sm" variant="primary" onclick={makeActive}>{t('servers.setAsActive')}</Button>
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
