<script lang="ts">
  import { AsyncBoundary, Empty, Section } from '@/ui'
  import { matches, useRoute } from '@/lib/router'
  import { t } from '@/lib/i18n/index'
  import { useGroups } from './resource.svelte'
  import GroupCard from './GroupCard.svelte'
  import GroupDetail from './GroupDetail.svelte'

  const res = useGroups()
  const route = useRoute()

  const showDetail = $derived.by(() => {
    void route.path
    return matches('/groups/:tag')
  })
</script>

{#if showDetail}
  <GroupDetail />
{:else}
  <Section
    title={t('nav.groups')}
    description={res.data ? t('groups.count', { n: res.data.length }) : undefined}
  >
    <AsyncBoundary resource={res}>
      {#snippet children(groups)}
        {#if groups.length === 0}
          <Empty icon="groups" title={t('groups.empty.title')} description={t('groups.empty.desc')} />
        {:else}
          <div class="grid">
            {#each groups as g (g.tag)}
              <GroupCard group={g} />
            {/each}
            <div class="stub">
              <div class="stub-inner">
                <span>+ {t('groups.newStub')}</span>
                <span class="hint">{t('groups.newStubHint')}</span>
              </div>
            </div>
          </div>
        {/if}
      {/snippet}
    </AsyncBoundary>
  </Section>
{/if}

<style>
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
    gap: var(--shuttle-space-3);
  }
  .stub {
    display: flex; align-items: center; justify-content: center;
    min-height: 180px;
    border: 2px dashed var(--shuttle-border-strong);
    border-radius: var(--shuttle-radius-md);
    color: var(--shuttle-fg-muted);
  }
  .stub-inner { text-align: center; display: flex; flex-direction: column; gap: var(--shuttle-space-1); font-size: var(--shuttle-text-sm); }
  .hint { font-size: var(--shuttle-text-xs); }
</style>
