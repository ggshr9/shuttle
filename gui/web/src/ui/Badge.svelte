<script lang="ts">
  import type { Snippet } from 'svelte'

  interface Props {
    variant?: 'neutral' | 'success' | 'warning' | 'danger' | 'info'
    children?: Snippet
  }
  let { variant = 'neutral', children }: Props = $props()
</script>

<span class="badge {variant}">{@render children?.()}</span>

<style>
  .badge {
    display: inline-flex;
    align-items: center;
    font-size: var(--shuttle-text-xs);
    font-weight: var(--shuttle-weight-medium);
    padding: 1px var(--shuttle-space-2);
    border-radius: var(--shuttle-radius-sm);
    letter-spacing: 0.02em;
    border: 1px solid var(--shuttle-border);
    color: var(--shuttle-fg-secondary);
    background: var(--shuttle-bg-subtle);
  }
  .success { color: var(--shuttle-success); border-color: color-mix(in oklab, var(--shuttle-success) 35%, transparent); }
  .warning { color: var(--shuttle-warning); border-color: color-mix(in oklab, var(--shuttle-warning) 35%, transparent); }
  .danger  { color: var(--shuttle-danger);  border-color: color-mix(in oklab, var(--shuttle-danger) 35%, transparent); }
  .info    { color: var(--shuttle-info);    border-color: color-mix(in oklab, var(--shuttle-info) 35%, transparent); }
</style>
