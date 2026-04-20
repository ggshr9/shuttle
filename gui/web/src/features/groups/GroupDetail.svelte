<script lang="ts">
  import { AsyncBoundary, Button, Icon, Section, StatRow, Badge } from '@/ui'
  import { useParams, navigate } from '@/lib/router'
  import { t } from '@/lib/i18n/index'
  import { useGroup, testGroup, selectMember, useGroupTestResults } from './resource.svelte'

  const params = $derived(useParams<{ tag: string }>('/groups/:tag'))
  const tag = $derived(decodeURIComponent(params.tag ?? ''))
  const res = $derived(useGroup(tag))

  const testRs = $derived(useGroupTestResults(tag))

  function latencyFor(member: string): string {
    const r = testRs.find((x) => x.tag === member)
    if (!r) return '— ms'
    if (!r.available) return t('groups.failed')
    return `${r.latency_ms} ms`
  }
</script>

<Section
  title={t('nav.groups')}
  description={tag}
>
  {#snippet actions()}
    <Button variant="ghost" onclick={() => navigate('/groups')}>
      <Icon name="chevronLeft" size={14} /> {t('groups.backToGroups')}
    </Button>
    <Button variant="secondary" onclick={() => testGroup(tag)}>{t('groups.testAll')}</Button>
  {/snippet}

  <AsyncBoundary resource={res}>
    {#snippet children(g)}
      <div class="summary">
        <StatRow label={t('groups.strategy')} value={g.strategy} mono />
        <StatRow label={t('groups.members')}  value={String(g.members.length)} />
        <StatRow label={t('groups.selected')} value={g.selected ?? '—'} mono />
      </div>

      <div class="members">
        {#each g.members as m}
          <div class="mrow">
            <span class="mname">{m}</span>
            <span class="mlat">{latencyFor(m)}</span>
            {#if g.selected === m}
              <Badge variant="success">{t('groups.active')}</Badge>
            {:else}
              <Button size="sm" variant="ghost" onclick={() => selectMember(tag, m)}>
                {t('groups.pick')}
              </Button>
            {/if}
          </div>
        {/each}
      </div>
    {/snippet}
  </AsyncBoundary>
</Section>

<style>
  .summary {
    display: grid; grid-template-columns: repeat(3, 1fr);
    gap: var(--shuttle-space-3);
    margin-bottom: var(--shuttle-space-5);
  }
  .members {
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    background: var(--shuttle-bg-surface);
    overflow: hidden;
  }
  .mrow {
    display: grid; grid-template-columns: 1fr 100px auto;
    align-items: center; gap: var(--shuttle-space-3);
    padding: var(--shuttle-space-2) var(--shuttle-space-4);
    border-top: 1px solid var(--shuttle-border);
    font-size: var(--shuttle-text-sm);
  }
  .mrow:first-child { border-top: 0; }
  .mname { font-family: var(--shuttle-font-mono); color: var(--shuttle-fg-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .mlat  { font-family: var(--shuttle-font-mono); color: var(--shuttle-fg-secondary); font-variant-numeric: tabular-nums; text-align: right; }
</style>
