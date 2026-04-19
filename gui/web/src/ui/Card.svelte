<script lang="ts">
  import type { Snippet } from 'svelte'

  interface Props {
    class?: string
    children?: Snippet
  }
  let { class: cls = '', children }: Props = $props()
</script>

<div class="card {cls}">{@render children?.()}</div>

<style>
  .card {
    background: var(--shuttle-bg-surface);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    padding: var(--shuttle-space-5);
  }
</style>
