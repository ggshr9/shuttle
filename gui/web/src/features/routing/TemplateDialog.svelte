<script lang="ts">
  import { Dialog, Button, AsyncBoundary } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { useTemplates, applyTemplate } from './resource.svelte'

  interface Props {
    open: boolean
    onApplied?: (id: string) => void
  }
  let { open = $bindable(false), onApplied }: Props = $props()

  const tpls = useTemplates()
  let selectedId = $state<string | null>(null)
  let busy = $state(false)

  async function apply() {
    if (!selectedId) return
    busy = true
    try {
      await applyTemplate(selectedId)
      onApplied?.(selectedId)
      open = false
    } finally {
      busy = false
    }
  }
</script>

<Dialog
  bind:open
  title={t('routing.templates.title')}
  description={t('routing.templates.desc')}
>
  <AsyncBoundary resource={tpls}>
    {#snippet children(list)}
      {#if list.length === 0}
        <p class="empty">{t('routing.templates.empty')}</p>
      {:else}
        <ul class="list">
          {#each list as tpl}
            <li>
              <label>
                <input
                  type="radio"
                  name="tpl"
                  value={tpl.id}
                  checked={selectedId === tpl.id}
                  onchange={() => (selectedId = tpl.id)}
                />
                <span class="name">{tpl.name}</span>
                <span class="desc">{tpl.description}</span>
              </label>
            </li>
          {/each}
        </ul>
      {/if}
    {/snippet}
  </AsyncBoundary>

  {#snippet actions()}
    <Button variant="ghost" onclick={() => (open = false)}>{t('common.cancel')}</Button>
    <Button variant="primary" disabled={!selectedId} loading={busy} onclick={apply}>
      {t('routing.templates.apply')}
    </Button>
  {/snippet}
</Dialog>

<style>
  .list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: var(--shuttle-space-2); }
  label {
    display: grid;
    grid-template-columns: 16px 1fr;
    gap: var(--shuttle-space-2);
    padding: var(--shuttle-space-2);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-sm);
    cursor: pointer;
  }
  label:has(input:checked) {
    border-color: var(--shuttle-accent);
    background: var(--shuttle-bg-subtle);
  }
  .name {
    font-weight: var(--shuttle-weight-medium);
    color: var(--shuttle-fg-primary);
    grid-row: 1; grid-column: 2;
  }
  .desc {
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-muted);
    grid-row: 2; grid-column: 2;
  }
  .empty { color: var(--shuttle-fg-muted); text-align: center; margin: 0; }
</style>
