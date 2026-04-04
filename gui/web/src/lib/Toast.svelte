<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { subscribe, dismiss, type ToastMessage } from './toast'

  let toasts = $state<ToastMessage[]>([])
  let unsubscribe: (() => void) | null = null

  onMount(() => {
    unsubscribe = subscribe((newToasts) => {
      toasts = newToasts
    })
  })

  onDestroy(() => {
    unsubscribe?.()
  })
</script>

<div class="toast-container" role="status" aria-live="polite">
  {#each toasts as t (t.id)}
    <div class="toast toast-{t.type}" role="alert">
      <span class="toast-icon">
        {#if t.type === 'success'}
          <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><path d="M8 1a7 7 0 100 14A7 7 0 008 1zm3.22 5.22a.75.75 0 10-1.06-1.06L7 8.34 5.84 7.16a.75.75 0 10-1.06 1.06l1.75 1.75a.75.75 0 001.06 0l3.63-3.75z"/></svg>
        {:else if t.type === 'error'}
          <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><path d="M8 1a7 7 0 100 14A7 7 0 008 1zm2.47 4.53a.75.75 0 010 1.06L9.06 8l1.41 1.41a.75.75 0 11-1.06 1.06L8 9.06l-1.41 1.41a.75.75 0 01-1.06-1.06L6.94 8 5.53 6.59a.75.75 0 011.06-1.06L8 6.94l1.41-1.41a.75.75 0 011.06 0z"/></svg>
        {:else if t.type === 'warning'}
          <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><path d="M8 1a7 7 0 100 14A7 7 0 008 1zm-.75 3.75a.75.75 0 011.5 0v3.5a.75.75 0 01-1.5 0v-3.5zM8 11a1 1 0 110 2 1 1 0 010-2z"/></svg>
        {:else}
          <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><path d="M8 1a7 7 0 100 14A7 7 0 008 1zm-.75 4.75a.75.75 0 011.5 0v4.5a.75.75 0 01-1.5 0v-4.5zM8 4a1 1 0 110-2 1 1 0 010 2z"/></svg>
        {/if}
      </span>
      <span class="toast-message">{t.message}</span>
      <button class="toast-close" onclick={() => dismiss(t.id)} aria-label="Dismiss">
        <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M4 4l6 6m0-6l-6 6"/></svg>
      </button>
    </div>
  {/each}
</div>

<style>
  .toast-container {
    position: fixed;
    top: 16px;
    right: 16px;
    z-index: 9999;
    display: flex;
    flex-direction: column;
    gap: 8px;
    max-width: 380px;
  }

  .toast {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 12px 16px;
    border-radius: var(--radius-md);
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    box-shadow: var(--shadow-lg);
    animation: slideIn 0.25s ease-out;
    backdrop-filter: blur(8px);
  }

  @keyframes slideIn {
    from {
      transform: translateX(100%);
      opacity: 0;
    }
    to {
      transform: translateX(0);
      opacity: 1;
    }
  }

  .toast-success {
    border-color: var(--accent-green);
    background: var(--bg-secondary);
  }

  .toast-error {
    border-color: var(--accent-red);
    background: var(--bg-secondary);
  }

  .toast-warning {
    border-color: var(--accent-yellow);
    background: var(--bg-secondary);
  }

  .toast-info {
    border-color: var(--accent);
    background: var(--bg-secondary);
  }

  .toast-icon {
    display: flex;
    align-items: center;
  }

  .toast-success .toast-icon { color: var(--accent-green); }
  .toast-error .toast-icon { color: var(--accent-red); }
  .toast-warning .toast-icon { color: var(--accent-yellow); }
  .toast-info .toast-icon { color: var(--accent); }

  .toast-message {
    flex: 1;
    font-size: 13px;
    color: var(--text-primary);
    line-height: 1.4;
  }

  .toast-close {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 2px;
    display: flex;
    align-items: center;
    border-radius: 4px;
    transition: color 0.15s;
  }

  .toast-close:hover {
    color: var(--text-primary);
  }
</style>
