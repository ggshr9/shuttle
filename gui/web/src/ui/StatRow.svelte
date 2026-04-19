<script lang="ts">
  interface Props {
    label: string
    value: string | number
    mono?: boolean
  }
  let { label, value, mono = false }: Props = $props()
</script>

<div class="row">
  <span class="lbl">{label}</span>
  <span class="val" class:mono>{value}</span>
</div>

<style>
  .row {
    display: flex; align-items: baseline; justify-content: space-between;
    font-size: var(--shuttle-text-sm);
    padding: var(--shuttle-space-1) 0;
  }
  .lbl { color: var(--shuttle-fg-secondary); }
  .val { color: var(--shuttle-fg-primary); font-variant-numeric: tabular-nums; font-weight: var(--shuttle-weight-medium); }
  .val.mono { font-family: var(--shuttle-font-mono); }
</style>
