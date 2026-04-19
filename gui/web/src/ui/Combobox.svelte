<script lang="ts" generics="V extends string">
  import { Combobox as BitsCombobox } from 'bits-ui'

  interface Item<V> { value: V; label: string }
  interface Props {
    value?: V
    items: Item<V>[]
    placeholder?: string
    onValueChange?: (v: V | undefined) => void
  }

  let { value = $bindable(), items, placeholder = 'Search…', onValueChange }: Props = $props()
  let input = $state('')
  const filtered = $derived(
    items.filter((it) => it.label.toLowerCase().includes(input.toLowerCase()))
  )
</script>

<BitsCombobox.Root
  type="single"
  bind:value
  onValueChange={(v) => onValueChange?.(v as V | undefined)}
>
  <BitsCombobox.Input
    oninput={(e) => (input = (e.target as HTMLInputElement).value)}
    class="shuttle-combobox-input"
    placeholder={placeholder}
  />
  <BitsCombobox.Portal>
    <BitsCombobox.Content class="shuttle-combobox-content">
      {#each filtered as it}
        <BitsCombobox.Item value={it.value} label={it.label} class="shuttle-combobox-item">{it.label}</BitsCombobox.Item>
      {/each}
    </BitsCombobox.Content>
  </BitsCombobox.Portal>
</BitsCombobox.Root>

<style>
  :global(.shuttle-combobox-input) {
    height: 32px; padding: 0 var(--shuttle-space-3);
    border: 1px solid var(--shuttle-border); border-radius: var(--shuttle-radius-md);
    background: var(--shuttle-bg-surface); color: var(--shuttle-fg-primary);
    font-size: var(--shuttle-text-base); font-family: var(--shuttle-font-sans);
    outline: none; min-width: 180px;
  }
  :global(.shuttle-combobox-input:focus) { border-color: var(--shuttle-border-strong); }
  :global(.shuttle-combobox-content) {
    background: var(--shuttle-bg-surface); border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md); box-shadow: var(--shuttle-shadow-md);
    padding: var(--shuttle-space-1); min-width: 180px; max-height: 240px; overflow-y: auto; z-index: 60;
    font-family: var(--shuttle-font-sans);
  }
  :global(.shuttle-combobox-item) {
    padding: var(--shuttle-space-1) var(--shuttle-space-3); border-radius: var(--shuttle-radius-sm);
    font-size: var(--shuttle-text-sm); color: var(--shuttle-fg-primary); cursor: pointer;
  }
  :global(.shuttle-combobox-item[data-highlighted]) { background: var(--shuttle-bg-subtle); }
</style>
