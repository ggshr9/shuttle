<script lang="ts">
  import { toasts, dismiss } from '@/lib/toaster.svelte'
  import { Icon } from '@/ui'
</script>

<div class="stack" role="status" aria-live="polite">
  {#each toasts.items as t (t.id)}
    <div class="toast {t.type}" role="alert">
      <span class="ico">
        <Icon name={t.type === 'success' ? 'check' : t.type === 'error' ? 'x' : 'info'} size={14} />
      </span>
      <span class="msg">{t.message}</span>
      <button class="close" onclick={() => dismiss(t.id)} aria-label="Close">
        <Icon name="x" size={12} />
      </button>
    </div>
  {/each}
</div>

<style>
  .stack {
    position: fixed;
    bottom: var(--shuttle-space-5);
    right: var(--shuttle-space-5);
    display: flex; flex-direction: column; gap: var(--shuttle-space-2);
    z-index: 80;
    pointer-events: none;
  }
  .toast {
    pointer-events: auto;
    display: flex; align-items: center; gap: var(--shuttle-space-2);
    min-width: 240px; max-width: 360px;
    padding: var(--shuttle-space-2) var(--shuttle-space-3);
    background: var(--shuttle-bg-surface);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    box-shadow: var(--shuttle-shadow-md);
    font-size: var(--shuttle-text-sm);
    color: var(--shuttle-fg-primary);
    font-family: var(--shuttle-font-sans);
  }
  .toast.success { border-left: 2px solid var(--shuttle-success); }
  .toast.error   { border-left: 2px solid var(--shuttle-danger); }
  .toast.warning { border-left: 2px solid var(--shuttle-warning); }
  .toast.info    { border-left: 2px solid var(--shuttle-info); }

  .ico { color: var(--shuttle-fg-secondary); display: inline-flex; }
  .msg { flex: 1; }
  .close {
    background: transparent; border: 0; padding: 2px;
    color: var(--shuttle-fg-muted); cursor: pointer;
    display: inline-flex;
  }
  .close:hover { color: var(--shuttle-fg-primary); }
</style>
