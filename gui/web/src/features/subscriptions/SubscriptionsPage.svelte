<script lang="ts">
  import { AsyncBoundary, Button, Icon, Section } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { useSubscriptions, deleteSubscription } from './resource.svelte'
  import SubscriptionTable from './SubscriptionTable.svelte'
  import AddSubscriptionDialog from './AddSubscriptionDialog.svelte'
  import DeleteConfirm from '@/features/servers/DeleteConfirm.svelte'

  const res = useSubscriptions()

  let addOpen = $state(false)
  let delOpen = $state(false)
  let pending = $state<string | null>(null)

  function openDelete(id: string) {
    pending = id
    delOpen = true
  }

  async function confirmDelete() {
    if (pending) await deleteSubscription(pending)
    pending = null
  }
</script>

<Section
  title={t('nav.subscriptions')}
  description={res.data ? t('subscriptions.count', { n: res.data.length }) : undefined}
>
  {#snippet actions()}
    <Button variant="primary" onclick={() => (addOpen = true)}>
      <Icon name="plus" size={14} /> {t('subscriptions.add')}
    </Button>
  {/snippet}

  <AsyncBoundary resource={res}>
    {#snippet children(items)}
      <SubscriptionTable {items} onDelete={openDelete} />
    {/snippet}
  </AsyncBoundary>
</Section>

<AddSubscriptionDialog bind:open={addOpen} />
<DeleteConfirm bind:open={delOpen} count={1} onConfirm={confirmDelete} />
