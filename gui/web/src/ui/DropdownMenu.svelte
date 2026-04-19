<script lang="ts">
  import type { Snippet } from 'svelte'
  import { DropdownMenu as BitsMenu } from 'bits-ui'

  interface MenuItem {
    label: string
    onselect: () => void
    danger?: boolean
    disabled?: boolean
  }
  interface Props {
    items: MenuItem[]
    children?: Snippet
  }
  let { items, children }: Props = $props()
</script>

<BitsMenu.Root>
  <BitsMenu.Trigger class="shuttle-menu-trigger">{@render children?.()}</BitsMenu.Trigger>
  <BitsMenu.Portal>
    <BitsMenu.Content class="shuttle-menu-content" sideOffset={4} align="end">
      {#each items as it}
        <BitsMenu.Item
          class={`shuttle-menu-item ${it.danger ? 'danger' : ''}`}
          disabled={it.disabled}
          onSelect={() => it.onselect()}
        >{it.label}</BitsMenu.Item>
      {/each}
    </BitsMenu.Content>
  </BitsMenu.Portal>
</BitsMenu.Root>

<style>
  :global(.shuttle-menu-trigger) {
    background: transparent; border: 0; padding: 0; cursor: pointer;
    color: inherit; font: inherit;
  }
  :global(.shuttle-menu-content) {
    background: var(--shuttle-bg-surface); border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md); box-shadow: var(--shuttle-shadow-md);
    padding: var(--shuttle-space-1); min-width: 160px; z-index: 60;
    font-family: var(--shuttle-font-sans);
  }
  :global(.shuttle-menu-item) {
    display: block; padding: var(--shuttle-space-1) var(--shuttle-space-3);
    border-radius: var(--shuttle-radius-sm); font-size: var(--shuttle-text-sm);
    color: var(--shuttle-fg-primary); cursor: pointer;
  }
  :global(.shuttle-menu-item[data-highlighted]) { background: var(--shuttle-bg-subtle); }
  :global(.shuttle-menu-item.danger) { color: var(--shuttle-danger); }
  :global(.shuttle-menu-item[data-disabled]) { color: var(--shuttle-fg-muted); pointer-events: none; }
</style>
