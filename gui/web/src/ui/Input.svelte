<script lang="ts">
  import type { HTMLInputAttributes } from 'svelte/elements'

  interface Props extends Omit<HTMLInputAttributes,
    'value' | 'type' | 'class' | 'oninput' | 'onchange' | 'placeholder' | 'disabled' | 'id' | 'autocomplete'
  > {
    value?: string
    placeholder?: string
    type?: 'text' | 'password' | 'email' | 'url' | 'number'
    label?: string
    error?: string
    disabled?: boolean
    oninput?: (e: Event) => void
    onchange?: (e: Event) => void
    class?: string
    id?: string
    autocomplete?: HTMLInputElement['autocomplete']
  }

  let {
    value = $bindable(''),
    placeholder = '',
    type = 'text',
    label,
    error,
    disabled = false,
    oninput,
    onchange,
    class: cls = '',
    id,
    autocomplete,
    ...rest
  }: Props = $props()

  const _fallbackId = `in-${Math.random().toString(36).slice(2, 8)}`
  const inputId = $derived(id ?? _fallbackId)
</script>

<div class="field {cls}" class:has-error={!!error}>
  {#if label}<label for={inputId}>{label}</label>{/if}
  <input
    id={inputId}
    {type}
    bind:value
    {placeholder}
    {disabled}
    {autocomplete}
    {oninput}
    {onchange}
    aria-invalid={!!error}
    aria-describedby={error ? `${inputId}-err` : undefined}
    {...rest}
  />
  {#if error}<p id={`${inputId}-err`} class="err">{error}</p>{/if}
</div>

<style>
  .field { display: flex; flex-direction: column; gap: var(--shuttle-space-1); }
  label {
    font-size: var(--shuttle-text-sm);
    color: var(--shuttle-fg-secondary);
    font-weight: var(--shuttle-weight-medium);
  }
  input {
    height: 32px;
    padding: 0 var(--shuttle-space-3);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    background: var(--shuttle-bg-surface);
    color: var(--shuttle-fg-primary);
    font-family: var(--shuttle-font-sans);
    font-size: var(--shuttle-text-base);
    outline: none;
    transition: border-color var(--shuttle-duration);
  }
  input:focus { border-color: var(--shuttle-border-strong); }
  input:disabled { opacity: 0.5; cursor: not-allowed; }
  .has-error input { border-color: var(--shuttle-danger); }
  .err {
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-danger);
    margin: 0;
  }
</style>
