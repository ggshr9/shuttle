<script lang="ts">
  import type { Snippet } from 'svelte'
  type Form = 'xs' | 'sm' | 'md' | 'lg' | 'xl'
  interface Props {
    cols?: Partial<Record<Form, number>>
    gap?: '1' | '2' | '3' | '4' | '5'
    children?: Snippet
  }
  let { cols = { xs: 1 }, gap = '3', children }: Props = $props()

  const styleVars = $derived.by(() => {
    const entries: string[] = []
    for (const [form, n] of Object.entries(cols)) {
      entries.push(`--cols-${form}: ${n}`)
    }
    return entries.join('; ')
  })
</script>

<div class="grid" data-gap={gap} style={styleVars}>
  {@render children?.()}
</div>

<style>
  .grid {
    display: grid;
    grid-template-columns: repeat(var(--cols-xs, 1), minmax(0, 1fr));
    gap: var(--shuttle-space-3);
  }
  .grid[data-gap="1"] { gap: var(--shuttle-space-1); }
  .grid[data-gap="2"] { gap: var(--shuttle-space-2); }
  .grid[data-gap="4"] { gap: var(--shuttle-space-4); }
  .grid[data-gap="5"] { gap: var(--shuttle-space-5); }

  @media (min-width: 480px)  { .grid { grid-template-columns: repeat(var(--cols-sm, var(--cols-xs, 1)), minmax(0, 1fr)); } }
  @media (min-width: 720px)  { .grid { grid-template-columns: repeat(var(--cols-md, var(--cols-sm, var(--cols-xs, 1))), minmax(0, 1fr)); } }
  @media (min-width: 1024px) { .grid { grid-template-columns: repeat(var(--cols-lg, var(--cols-md, var(--cols-sm, var(--cols-xs, 1)))), minmax(0, 1fr)); } }
  @media (min-width: 1440px) { .grid { grid-template-columns: repeat(var(--cols-xl, var(--cols-lg, var(--cols-md, var(--cols-sm, var(--cols-xs, 1))))), minmax(0, 1fr)); } }
</style>
