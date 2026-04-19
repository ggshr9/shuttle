<script lang="ts">
  import type { Snippet } from 'svelte'

  interface Props {
    title?: string
    description?: string
    actions?: Snippet
    children?: Snippet
  }
  let { title, description, actions, children }: Props = $props()
</script>

<section class="sec">
  {#if title || actions}
    <header>
      <div>
        {#if title}<h3>{title}</h3>{/if}
        {#if description}<p>{description}</p>{/if}
      </div>
      {#if actions}<div class="actions">{@render actions()}</div>{/if}
    </header>
  {/if}
  <div class="body">{@render children?.()}</div>
</section>

<style>
  .sec { margin-bottom: var(--shuttle-space-6); }
  header { display: flex; align-items: flex-start; justify-content: space-between; margin-bottom: var(--shuttle-space-3); gap: var(--shuttle-space-4); }
  h3 { margin: 0; font-size: var(--shuttle-text-lg); font-weight: var(--shuttle-weight-semibold); letter-spacing: var(--shuttle-tracking-tight); color: var(--shuttle-fg-primary); }
  p { margin: var(--shuttle-space-1) 0 0; font-size: var(--shuttle-text-sm); color: var(--shuttle-fg-secondary); }
</style>
