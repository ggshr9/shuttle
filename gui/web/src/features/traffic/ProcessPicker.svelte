<script lang="ts">
  import { Dialog, Button, Combobox } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { useProcesses } from './resource.svelte'

  interface Props {
    open: boolean
    onPick: (name: string) => void
  }
  let { open = $bindable(false), onPick }: Props = $props()

  const procs = useProcesses()
  let picked = $state<string | undefined>(undefined)

  function confirm() {
    if (picked) {
      onPick(picked)
      open = false
      picked = undefined
    }
  }
</script>

<Dialog bind:open title={t('routing.process.title')} description={t('routing.process.desc')}>
  {#if procs.data && procs.data.length > 0}
    <Combobox
      value={picked}
      items={procs.data.map((p) => ({ value: p.name, label: `${p.name} (${p.conns})` }))}
      onValueChange={(v) => (picked = v ?? undefined)}
    />
  {:else}
    <p class="hint">{t('routing.process.none')}</p>
  {/if}

  {#snippet actions()}
    <Button variant="ghost" onclick={() => (open = false)}>{t('common.cancel')}</Button>
    <Button variant="primary" disabled={!picked} onclick={confirm}>{t('routing.process.pick')}</Button>
  {/snippet}
</Dialog>

<style>
  .hint { color: var(--shuttle-fg-muted); font-size: var(--shuttle-text-sm); text-align: center; margin: 0; }
</style>
