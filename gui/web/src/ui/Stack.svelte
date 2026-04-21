<script lang="ts">
  import type { Snippet } from 'svelte'
  type Form = 'xs' | 'sm' | 'md' | 'lg' | 'xl'
  interface Props {
    gap?: '1' | '2' | '3' | '4' | '5'
    direction?: 'row' | 'column'
    wrap?: boolean
    breakAt?: Form                 // when set, direction is 'row' at >= breakAt, 'column' below
    children?: Snippet
  }
  let { gap = '3', direction = 'column', wrap = false, breakAt, children }: Props = $props()
</script>

<div
  class="stack"
  data-direction={direction}
  data-gap={gap}
  data-wrap={wrap ? '1' : '0'}
  data-break-at={breakAt ?? ''}
>
  {@render children?.()}
</div>

<style>
  .stack {
    display: flex;
    flex-direction: var(--_dir, column);
    gap: var(--shuttle-space-3);
    flex-wrap: nowrap;
  }
  .stack[data-direction="row"] { --_dir: row; }
  .stack[data-gap="1"] { gap: var(--shuttle-space-1); }
  .stack[data-gap="2"] { gap: var(--shuttle-space-2); }
  .stack[data-gap="4"] { gap: var(--shuttle-space-4); }
  .stack[data-gap="5"] { gap: var(--shuttle-space-5); }
  .stack[data-wrap="1"] { flex-wrap: wrap; }

  /* breakAt variants — direction flips at the named breakpoint upward */
  @media (min-width: 480px)  { .stack[data-break-at="sm"]:not([data-direction="row"]) { flex-direction: row; } }
  @media (min-width: 720px)  { .stack[data-break-at="md"]:not([data-direction="row"]) { flex-direction: row; } }
  @media (min-width: 1024px) { .stack[data-break-at="lg"]:not([data-direction="row"]) { flex-direction: row; } }
</style>
