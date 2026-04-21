<script lang="ts">
  interface Props { title: string }
  let { title }: Props = $props()
</script>

<h2>{title}</h2>

<style>
  h2 {
    margin: 0 0 var(--shuttle-space-4);
    font-size: var(--shuttle-text-lg);
    font-weight: var(--shuttle-weight-semibold);
    color: var(--shuttle-fg-primary);
  }
</style>
