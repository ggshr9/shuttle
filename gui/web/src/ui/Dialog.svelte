<script lang="ts">
  import type { Snippet } from 'svelte'
  import { Dialog as BitsDialog } from 'bits-ui'

  interface Props {
    open?: boolean
    onOpenChange?: (open: boolean) => void
    title: string
    description?: string
    children?: Snippet
    actions?: Snippet
  }

  let { open = $bindable(false), onOpenChange, title, description, children, actions }: Props = $props()
</script>

<BitsDialog.Root bind:open onOpenChange={onOpenChange}>
  <BitsDialog.Portal>
    <BitsDialog.Overlay class="shuttle-dialog-overlay" />
    <BitsDialog.Content class="shuttle-dialog-content">
      <BitsDialog.Title class="shuttle-dialog-title">{title}</BitsDialog.Title>
      {#if description}
        <BitsDialog.Description class="shuttle-dialog-desc">{description}</BitsDialog.Description>
      {/if}
      <div class="body">{@render children?.()}</div>
      {#if actions}<div class="actions">{@render actions()}</div>{/if}
    </BitsDialog.Content>
  </BitsDialog.Portal>
</BitsDialog.Root>

<style>
  :global(.shuttle-dialog-overlay) {
    position: fixed; inset: 0; background: rgba(0, 0, 0, 0.5);
    z-index: 50;
  }
  :global(.shuttle-dialog-content) {
    position: fixed; top: 50%; left: 50%; transform: translate(-50%, -50%);
    background: var(--shuttle-bg-surface);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-lg);
    padding: var(--shuttle-space-5);
    min-width: 360px; max-width: 90vw; max-height: 85vh; overflow: auto;
    z-index: 51;
    color: var(--shuttle-fg-primary);
    font-family: var(--shuttle-font-sans);
  }
  :global(.shuttle-dialog-title) {
    margin: 0; font-size: var(--shuttle-text-lg); font-weight: var(--shuttle-weight-semibold);
    color: var(--shuttle-fg-primary);
  }
  :global(.shuttle-dialog-desc) {
    margin: var(--shuttle-space-1) 0 0; font-size: var(--shuttle-text-sm);
    color: var(--shuttle-fg-secondary);
  }
  .body { margin-top: var(--shuttle-space-4); }
  .actions { margin-top: var(--shuttle-space-5); display: flex; justify-content: flex-end; gap: var(--shuttle-space-2); }
</style>
