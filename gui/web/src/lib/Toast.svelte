<script>
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
        {#if t.type === 'success'}✓{:else if t.type === 'error'}✕{:else if t.type === 'warning'}⚠{:else}ℹ{/if}
      </span>
      <span class="toast-message">{t.message}</span>
      <button class="toast-close" onclick={() => dismiss(t.id)} aria-label="Dismiss">×</button>
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
    max-width: 360px;
  }

  .toast {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 12px 16px;
    border-radius: 8px;
    background: #161b22;
    border: 1px solid #2d333b;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
    animation: slideIn 0.2s ease-out;
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
    border-color: #238636;
    background: linear-gradient(135deg, #161b22 0%, #1a2f1a 100%);
  }

  .toast-error {
    border-color: #f85149;
    background: linear-gradient(135deg, #161b22 0%, #2f1a1a 100%);
  }

  .toast-warning {
    border-color: #d29922;
    background: linear-gradient(135deg, #161b22 0%, #2f2a1a 100%);
  }

  .toast-info {
    border-color: #58a6ff;
    background: linear-gradient(135deg, #161b22 0%, #1a1f2f 100%);
  }

  .toast-icon {
    font-size: 16px;
    font-weight: bold;
  }

  .toast-success .toast-icon { color: #3fb950; }
  .toast-error .toast-icon { color: #f85149; }
  .toast-warning .toast-icon { color: #d29922; }
  .toast-info .toast-icon { color: #58a6ff; }

  .toast-message {
    flex: 1;
    font-size: 14px;
    color: #e1e4e8;
    line-height: 1.4;
  }

  .toast-close {
    background: none;
    border: none;
    color: #8b949e;
    font-size: 18px;
    cursor: pointer;
    padding: 0 4px;
    line-height: 1;
  }

  .toast-close:hover {
    color: #e1e4e8;
  }
</style>
