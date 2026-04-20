<script lang="ts">
  import { StatRow } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import type { Subscription } from '@/lib/api/types'

  interface Props { sub: Subscription }
  let { sub }: Props = $props()

  const shown = $derived((sub.servers ?? []).slice(0, 5))
  const more  = $derived(Math.max(0, (sub.servers?.length ?? 0) - 5))
</script>

<div class="pane">
  <div class="fields">
    <StatRow label={t('subscriptions.columns.url')} value={sub.url} mono />
    <StatRow label={t('subscriptions.columns.servers')} value={String(sub.servers?.length ?? 0)} />
    <StatRow label={t('subscriptions.columns.updated')} value={sub.updated_at ?? '—'} mono />
    {#if sub.error}
      <div class="err">{sub.error}</div>
    {/if}
  </div>

  {#if shown.length > 0}
    <div class="servers">
      <h4>{t('subscriptions.importedServers')}</h4>
      <ul>
        {#each shown as s}
          <li>
            <span class="sname">{s.name || s.addr}</span>
            <span class="saddr">{s.addr}</span>
          </li>
        {/each}
        {#if more > 0}
          <li class="more">{t('subscriptions.andMore', { n: more })}</li>
        {/if}
      </ul>
    </div>
  {/if}
</div>

<style>
  .pane {
    display: grid; grid-template-columns: 1fr 280px;
    gap: var(--shuttle-space-5);
    padding: var(--shuttle-space-4) var(--shuttle-space-4) var(--shuttle-space-4) var(--shuttle-space-6);
    background: var(--shuttle-bg-subtle);
    border-top: 1px solid var(--shuttle-border);
  }
  .fields { display: flex; flex-direction: column; gap: var(--shuttle-space-2); }
  .err {
    padding: var(--shuttle-space-2);
    background: color-mix(in oklab, var(--shuttle-danger) 10%, transparent);
    color: var(--shuttle-danger);
    border-radius: var(--shuttle-radius-sm);
    font-size: var(--shuttle-text-xs);
    font-family: var(--shuttle-font-mono);
  }
  .servers h4 {
    margin: 0 0 var(--shuttle-space-2);
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-muted);
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }
  ul { list-style: none; margin: 0; padding: 0; }
  li {
    display: flex; justify-content: space-between;
    padding: var(--shuttle-space-1) 0;
    font-size: var(--shuttle-text-xs);
  }
  .sname { color: var(--shuttle-fg-primary); }
  .saddr { color: var(--shuttle-fg-muted); font-family: var(--shuttle-font-mono); }
  .more  { color: var(--shuttle-fg-muted); font-style: italic; }
</style>
