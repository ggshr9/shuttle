<script lang="ts">
  import type { Snippet } from 'svelte'
  import { Tooltip as BitsTooltip } from 'bits-ui'

  interface Props {
    content: string
    side?: 'top' | 'bottom' | 'left' | 'right'
    children?: Snippet
  }
  let { content, side = 'top', children }: Props = $props()
</script>

<BitsTooltip.Provider delayDuration={200}>
  <BitsTooltip.Root>
    <BitsTooltip.Trigger class="shuttle-tooltip-trigger">{@render children?.()}</BitsTooltip.Trigger>
    <BitsTooltip.Portal>
      <BitsTooltip.Content {side} sideOffset={6} class="shuttle-tooltip-content">{content}</BitsTooltip.Content>
    </BitsTooltip.Portal>
  </BitsTooltip.Root>
</BitsTooltip.Provider>

<style>
  :global(.shuttle-tooltip-trigger) {
    display: inline-flex; background: transparent; border: 0; padding: 0; cursor: inherit;
    color: inherit; font: inherit;
  }
  :global(.shuttle-tooltip-content) {
    background: var(--shuttle-fg-primary); color: var(--shuttle-bg-base);
    padding: 4px 8px; border-radius: var(--shuttle-radius-sm);
    font-size: var(--shuttle-text-xs); z-index: 70;
    font-family: var(--shuttle-font-sans);
  }
</style>
