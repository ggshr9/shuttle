<script lang="ts">
  import type { Snippet } from 'svelte'

  interface Props {
    label: string
    hint?: string
    children: Snippet
  }
  let { label, hint, children }: Props = $props()
</script>

<div class="field">
  <div class="meta">
    <div class="label">{label}</div>
    {#if hint}<div class="hint">{hint}</div>{/if}
  </div>
  <div class="control">{@render children()}</div>
</div>

<style>
  .field {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: var(--shuttle-space-4);
    padding: var(--shuttle-space-3) 0;
    border-bottom: 1px solid var(--shuttle-border);
    align-items: center;
  }
  .field:last-child { border-bottom: none; }
  .meta { min-width: 0; }
  .label {
    font-size: var(--shuttle-text-sm);
    color: var(--shuttle-fg-primary);
    font-weight: var(--shuttle-weight-medium);
  }
  .hint {
    margin-top: var(--shuttle-space-1);
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-muted);
  }
  .control { display: flex; justify-content: flex-end; align-items: center; gap: var(--shuttle-space-2); }
</style>
