<script lang="ts">
  import type { Snippet } from 'svelte'

  interface Props {
    type?: 'button' | 'submit' | 'reset'
    variant?: 'primary' | 'secondary' | 'ghost' | 'danger'
    size?: 'sm' | 'md'
    loading?: boolean
    disabled?: boolean
    onclick?: (e: MouseEvent) => void
    class?: string
    children?: Snippet
  }

  let {
    type = 'button',
    variant = 'secondary',
    size = 'md',
    loading = false,
    disabled = false,
    onclick,
    class: cls = '',
    children,
  }: Props = $props()
</script>

<button
  {type}
  class="btn btn-{variant} btn-{size} {cls}"
  class:loading
  disabled={disabled || loading}
  onclick={(e) => { if (!disabled && !loading) onclick?.(e) }}
>
  {#if loading}<span class="spinner" aria-hidden="true"></span>{/if}
  {@render children?.()}
</button>

<style>
  .btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--shuttle-space-2);
    border: 1px solid transparent;
    border-radius: var(--shuttle-radius-md);
    font-family: var(--shuttle-font-sans);
    font-weight: var(--shuttle-weight-medium);
    cursor: pointer;
    transition: background var(--shuttle-duration) var(--shuttle-easing),
                border-color var(--shuttle-duration) var(--shuttle-easing),
                color var(--shuttle-duration) var(--shuttle-easing);
    white-space: nowrap;
  }
  .btn-md { height: 32px; padding: 0 var(--shuttle-space-3); font-size: var(--shuttle-text-base); }
  .btn-sm { height: 26px; padding: 0 var(--shuttle-space-2); font-size: var(--shuttle-text-sm); }

  .btn-primary {
    background: var(--shuttle-accent);
    color: var(--shuttle-accent-fg);
  }
  .btn-primary:hover:not(:disabled) { background: var(--shuttle-fg-primary); }

  .btn-secondary {
    background: var(--shuttle-bg-surface);
    color: var(--shuttle-fg-primary);
    border-color: var(--shuttle-border);
  }
  .btn-secondary:hover:not(:disabled) { border-color: var(--shuttle-border-strong); background: var(--shuttle-bg-subtle); }

  .btn-ghost {
    background: transparent;
    color: var(--shuttle-fg-secondary);
  }
  .btn-ghost:hover:not(:disabled) { background: var(--shuttle-bg-subtle); color: var(--shuttle-fg-primary); }

  .btn-danger {
    background: var(--shuttle-danger);
    color: #fff;
  }
  .btn-danger:hover:not(:disabled) { filter: brightness(1.05); }

  .btn:disabled { opacity: 0.5; cursor: not-allowed; }

  .spinner {
    width: 12px; height: 12px;
    border: 1.5px solid currentColor;
    border-right-color: transparent;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }
  @keyframes spin { to { transform: rotate(360deg); } }
</style>
