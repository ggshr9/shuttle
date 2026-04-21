<script lang="ts">
  interface Props {
    total: number
    current: number  // 1-based
  }
  let { total, current }: Props = $props()

  const dots = $derived(Array.from({ length: total }, (_, i) => i + 1))
</script>

<div class="dots" role="progressbar" aria-valuemin="1" aria-valuemax={total} aria-valuenow={current}>
  {#each dots as n (n)}
    <span class="dot" class:on={n <= current}></span>
  {/each}
</div>

<style>
  .dots {
    display: flex;
    gap: var(--shuttle-space-2);
    justify-content: center;
  }
  .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--shuttle-border);
    transition: background 200ms ease;
  }
  .dot.on { background: var(--shuttle-fg-primary); }
</style>
