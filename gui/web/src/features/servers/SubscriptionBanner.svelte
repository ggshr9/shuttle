<script lang="ts">
  import { Button } from '@/ui'
  import type { Subscription } from '@/lib/api/types'
  import { refreshSubscription, deleteSubscription } from '@/lib/api/endpoints'
  import { invalidate } from '@/lib/resource.svelte'
  import { toasts } from '@/lib/toaster.svelte'
  import { errorMessage } from '@/lib/format'

  interface Props { sub: Subscription }
  let { sub }: Props = $props()
  let busy = $state(false)

  const serverCount = $derived(sub.servers?.length ?? 0)
  const lastRefresh = $derived(
    sub.updated_at ? new Date(sub.updated_at).toLocaleString() : 'never'
  )

  async function refresh() {
    busy = true
    try {
      await refreshSubscription(sub.id)
      invalidate('subscriptions.list')
      invalidate('servers.list')
      toasts.success('Subscription refreshed')
    } catch (e) {
      toasts.error(errorMessage(e))
    } finally { busy = false }
  }

  async function remove() {
    if (!confirm(`Delete subscription ${sub.name}?`)) return
    busy = true
    try {
      await deleteSubscription(sub.id)
      invalidate('subscriptions.list')
      invalidate('servers.list')
    } catch (e) {
      toasts.error(errorMessage(e))
    } finally { busy = false }
  }
</script>

<div class="banner">
  <div class="meta">
    <div class="name">{sub.name}</div>
    <div class="url" title={sub.url}>{sub.url}</div>
    <div class="stats">
      {serverCount} servers · last refresh {lastRefresh}
    </div>
    {#if sub.error}
      <div class="error">{sub.error}</div>
    {/if}
  </div>
  <div class="actions">
    <Button size="sm" loading={busy} onclick={refresh}>Refresh</Button>
    <Button size="sm" variant="ghost" onclick={remove}>Delete</Button>
  </div>
</div>

<style>
  .banner {
    display: flex; justify-content: space-between; align-items: flex-start;
    gap: var(--shuttle-space-3);
    padding: var(--shuttle-space-3);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    margin-bottom: var(--shuttle-space-3);
    background: var(--shuttle-bg-subtle);
  }
  .meta { min-width: 0; flex: 1; }
  .name {
    font-weight: var(--shuttle-weight-semibold);
    color: var(--shuttle-fg-primary);
  }
  .url {
    color: var(--shuttle-fg-muted);
    font-size: var(--shuttle-text-sm);
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
    font-family: var(--shuttle-font-mono);
  }
  .stats {
    color: var(--shuttle-fg-muted);
    font-size: var(--shuttle-text-xs);
    margin-top: var(--shuttle-space-1);
  }
  .error {
    color: var(--shuttle-danger, #da3633);
    font-size: var(--shuttle-text-xs);
    margin-top: var(--shuttle-space-1);
  }
  .actions {
    display: flex; gap: var(--shuttle-space-1);
    flex-shrink: 0;
  }
</style>
