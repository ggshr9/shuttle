<script lang="ts">
  import { Switch as BitsSwitch } from 'bits-ui'

  interface Props {
    checked?: boolean
    disabled?: boolean
    onCheckedChange?: (checked: boolean) => void
    label?: string
    id?: string
  }

  let {
    checked = $bindable(false),
    disabled = false,
    onCheckedChange,
    label,
    id,
  }: Props = $props()

  // Stable fallback (computed once at mount); derived picks id when parent
  // passes it, otherwise uses fallback. Reactive so an `id` prop change is
  // respected.
  const _fallbackId = `sw-${Math.random().toString(36).slice(2, 8)}`
  const switchId = $derived(id ?? _fallbackId)
</script>

<div class="wrap">
  <BitsSwitch.Root
    id={switchId}
    bind:checked
    {disabled}
    onCheckedChange={onCheckedChange}
    class="shuttle-switch-root"
  >
    <BitsSwitch.Thumb class="shuttle-switch-thumb" />
  </BitsSwitch.Root>
  {#if label}<label for={switchId}>{label}</label>{/if}
</div>

<style>
  .wrap { display: inline-flex; align-items: center; gap: var(--shuttle-space-2); }
  label { font-size: var(--shuttle-text-sm); color: var(--shuttle-fg-secondary); cursor: pointer; }
  :global(.shuttle-switch-root) {
    width: 32px; height: 18px; border-radius: 999px;
    background: var(--shuttle-bg-subtle); border: 1px solid var(--shuttle-border);
    position: relative; cursor: pointer; transition: background var(--shuttle-duration);
  }
  :global(.shuttle-switch-root[data-state="checked"]) { background: var(--shuttle-accent); }
  :global(.shuttle-switch-thumb) {
    display: block; width: 14px; height: 14px; border-radius: 999px;
    background: var(--shuttle-fg-secondary); transform: translateX(0);
    transition: transform var(--shuttle-duration), background var(--shuttle-duration);
    position: absolute; top: 1px; left: 1px;
  }
  :global(.shuttle-switch-root[data-state="checked"] .shuttle-switch-thumb) {
    transform: translateX(14px); background: var(--shuttle-accent-fg);
  }
</style>
