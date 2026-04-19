<script lang="ts" generics="V extends string">
  import type { Snippet } from 'svelte'
  import { Tabs as BitsTabs } from 'bits-ui'

  interface Item<V> { value: V; label: string }
  interface Props {
    value?: V
    items: Item<V>[]
    onValueChange?: (v: V) => void
    children?: Snippet<[V]>
  }

  let { value = $bindable(), items, onValueChange, children }: Props = $props()
</script>

<BitsTabs.Root bind:value onValueChange={(v) => onValueChange?.(v as V)}>
  <BitsTabs.List class="shuttle-tabs-list">
    {#each items as it}
      <BitsTabs.Trigger value={it.value} class="shuttle-tabs-trigger">{it.label}</BitsTabs.Trigger>
    {/each}
  </BitsTabs.List>
  {#if children && value !== undefined}
    {@render children(value as V)}
  {/if}
</BitsTabs.Root>

<style>
  :global(.shuttle-tabs-list) {
    display: inline-flex; gap: var(--shuttle-space-1);
    background: var(--shuttle-bg-subtle); padding: 2px; border-radius: var(--shuttle-radius-md);
  }
  :global(.shuttle-tabs-trigger) {
    padding: var(--shuttle-space-1) var(--shuttle-space-3);
    border: 0; background: transparent;
    font-size: var(--shuttle-text-sm); font-weight: var(--shuttle-weight-medium); color: var(--shuttle-fg-secondary);
    border-radius: var(--shuttle-radius-sm); cursor: pointer;
    font-family: var(--shuttle-font-sans);
  }
  :global(.shuttle-tabs-trigger[data-state="active"]) {
    background: var(--shuttle-bg-surface); color: var(--shuttle-fg-primary); box-shadow: var(--shuttle-shadow-sm);
  }
</style>
