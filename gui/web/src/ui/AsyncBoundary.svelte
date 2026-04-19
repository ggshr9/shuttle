<script lang="ts" generics="T">
  import type { Snippet } from 'svelte'
  import Spinner from './Spinner.svelte'
  import ErrorBanner from './ErrorBanner.svelte'
  import Empty from './Empty.svelte'
  import type { Resource } from '@/lib/resource.svelte'

  interface Props {
    resource: Resource<T>
    emptyTitle?: string
    emptyDescription?: string
    isEmpty?: (data: T) => boolean
    children: Snippet<[T]>
  }

  let { resource, emptyTitle, emptyDescription, isEmpty, children }: Props = $props()
</script>

{#if resource.loading && resource.data === undefined}
  <div class="center"><Spinner size={20} /></div>
{:else if resource.error && resource.data === undefined}
  <ErrorBanner message={resource.error.message} onretry={() => resource.refetch()} />
{:else if resource.data !== undefined && isEmpty?.(resource.data)}
  <Empty title={emptyTitle ?? 'Nothing here'} description={emptyDescription} />
{:else if resource.data !== undefined}
  {@render children(resource.data)}
{/if}

<style>
  .center { display: flex; justify-content: center; padding: var(--shuttle-space-6); }
</style>
