<script lang="ts" generics="V extends string">
  import { Select as BitsSelect } from 'bits-ui'
  import Icon from './Icon.svelte'

  interface Option<V> { value: V; label: string }
  interface Props {
    value?: V
    options: Option<V>[]
    placeholder?: string
    disabled?: boolean
    onValueChange?: (v: V) => void
  }

  let {
    value = $bindable(),
    options,
    placeholder = 'Select…',
    disabled = false,
    onValueChange,
  }: Props = $props()
</script>

<BitsSelect.Root
  type="single"
  bind:value
  {disabled}
  onValueChange={(v) => onValueChange?.(v as V)}
>
  <BitsSelect.Trigger class="shuttle-select-trigger">
    <span>{options.find((o) => o.value === value)?.label ?? placeholder}</span>
    <Icon name="chevronDown" size={14} />
  </BitsSelect.Trigger>
  <BitsSelect.Portal>
    <BitsSelect.Content class="shuttle-select-content" sideOffset={4}>
      {#each options as o}
        <BitsSelect.Item value={o.value} label={o.label} class="shuttle-select-item">
          {o.label}
        </BitsSelect.Item>
      {/each}
    </BitsSelect.Content>
  </BitsSelect.Portal>
</BitsSelect.Root>

<style>
  :global(.shuttle-select-trigger) {
    display: inline-flex; align-items: center; justify-content: space-between; gap: var(--shuttle-space-2);
    height: 32px; padding: 0 var(--shuttle-space-3);
    background: var(--shuttle-bg-surface); color: var(--shuttle-fg-primary);
    border: 1px solid var(--shuttle-border); border-radius: var(--shuttle-radius-md);
    font-size: var(--shuttle-text-base); font-family: var(--shuttle-font-sans); cursor: pointer; min-width: 140px;
  }
  :global(.shuttle-select-trigger:hover) { border-color: var(--shuttle-border-strong); }
  :global(.shuttle-select-content) {
    background: var(--shuttle-bg-surface); border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md); box-shadow: var(--shuttle-shadow-md);
    min-width: 140px; z-index: 60; padding: var(--shuttle-space-1);
  }
  :global(.shuttle-select-item) {
    padding: var(--shuttle-space-1) var(--shuttle-space-3); font-size: var(--shuttle-text-sm);
    border-radius: var(--shuttle-radius-sm); cursor: pointer; color: var(--shuttle-fg-primary);
  }
  :global(.shuttle-select-item[data-highlighted]) { background: var(--shuttle-bg-subtle); }
</style>
