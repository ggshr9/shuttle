<script lang="ts">
  import { AsyncBoundary, Button, Icon, Section } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { useServers, removeServer, removeMany, autoSelect, runSpeedtest, useSubscriptions, useGroups } from './resource.svelte'
  import ServerTable from './ServerTable.svelte'
  import AddSheet from './AddSheet.svelte'
  import DeleteConfirm from './DeleteConfirm.svelte'
  import SourceFilter from './SourceFilter.svelte'
  import SubscriptionBanner from './SubscriptionBanner.svelte'
  import { useRoute, navigate } from '@/lib/router'
  import type { Server, Subscription } from '@/lib/api/types'

  const res = useServers()
  const subsRes = useSubscriptions()
  const groupsRes = useGroups()

  let selected = $state<Set<string>>(new Set())
  let addOpen = $state(false)
  let deleteOpen = $state(false)
  let pendingDelete = $state<string[]>([])

  const route = useRoute()

  const currentFilter = $derived.by(() => {
    if (route.query.source) return route.query.source
    if (route.query.group) return `group:${route.query.group}`
    return 'all'
  })

  const activeSubId = $derived(
    currentFilter.startsWith('subscription:')
      ? currentFilter.slice('subscription:'.length)
      : null
  )

  const activeSub = $derived<Subscription | null>(
    activeSubId ? (subsRes.data?.find((s) => s.id === activeSubId) ?? null) : null
  )

  function setFilter(v: string) {
    if (v === 'all') {
      navigate('/servers')
    } else if (v.startsWith('group:')) {
      navigate(`/servers?group=${v.slice('group:'.length)}`)
    } else {
      navigate(`/servers?source=${v}`)
    }
  }

  // Build the set of addrs that belong to any subscription — used by the
  // 'manual' and 'subscriptions' filters.
  const subServerAddrs = $derived<Set<string>>(new Set(
    (subsRes.data ?? []).flatMap((s) => s.servers ?? []).map((s) => s.addr)
  ))

  function matchesFilter(srv: Server): boolean {
    const f = currentFilter
    if (f === 'all') return true
    if (f === 'manual') return !subServerAddrs.has(srv.addr)
    if (f === 'subscriptions') return subServerAddrs.has(srv.addr)
    if (f.startsWith('subscription:')) {
      const id = f.slice('subscription:'.length)
      const sub = subsRes.data?.find((s) => s.id === id)
      return !!sub?.servers?.some((s) => s.addr === srv.addr)
    }
    if (f.startsWith('group:')) {
      const groupTag = f.slice('group:'.length)
      const group = groupsRes.data?.find((g) => g.tag === groupTag)
      return !!group?.members?.includes(srv.addr)
    }
    return true
  }

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
  title={t('nav.servers')}
  description={res.data ? (
    res.data.servers.length === 1
      ? t('servers.configured_one',   { n: res.data.servers.length })
      : t('servers.configured_other', { n: res.data.servers.length })
  ) : undefined}
>
  {#snippet actions()}
    <Button variant="ghost" onclick={testAll}>{t('servers.testAll')}</Button>
    <Button variant="ghost" onclick={() => autoSelect()}>
      <Icon name="check" size={14} /> {t('servers.autoSelect')}
    </Button>
    <Button variant="primary" onclick={() => (addOpen = true)}>
      <Icon name="plus" size={14} /> {t('servers.addServer')}
    </Button>
  {/snippet}

  <SourceFilter
    value={currentFilter}
    sources={[
      { id: 'manual', label: t('servers.sourceManual') },
      { id: 'subscriptions', label: t('servers.sourceSubs') },
      ...(subsRes.data ?? []).map((s) => ({
        id: `subscription:${s.id}`,
        label: s.name,
      })),
    ]}
    groups={(groupsRes.data ?? []).map((g) => ({ id: g.tag, label: g.tag }))}
    onChange={setFilter}
  />

  {#if activeSub}
    <SubscriptionBanner sub={activeSub} />
  {/if}

  {#if selected.size > 0}
    <div class="sel-bar">
      <span class="count">{t('servers.selected', { n: selected.size })}</span>
      <Button size="sm" variant="secondary" onclick={testSelected}>{t('servers.speedTest')}</Button>
      <Button size="sm" variant="danger"    onclick={openBatchDelete}>{t('common.delete')}</Button>
      <Button size="sm" variant="ghost"     onclick={() => (selected = new Set())}>{t('common.cancel')}</Button>
    </div>
  {/if}

  <AsyncBoundary resource={res}>
    {#snippet children(list)}
      <ServerTable
        servers={list.servers.filter(matchesFilter)}
        activeAddr={list.active.addr}
        {selected}
        onSelectedChange={(s) => (selected = s)}
        onDelete={openSingleDelete}
      />
    {/snippet}
  </AsyncBoundary>
</Section>

<AddSheet bind:open={addOpen} />
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
