<script lang="ts">
  import Icon from './Icon.svelte'
  interface Props {
    message: string
    onretry?: () => void
    retryLabel?: string
  }
  let { message, onretry, retryLabel = 'Retry' }: Props = $props()
</script>

<div class="banner" role="alert">
  <div class="msg"><Icon name="info" size={14} />{message}</div>
  {#if onretry}<button class="retry" onclick={onretry}>{retryLabel}</button>{/if}
</div>

<style>
  .banner {
    display: flex; align-items: center; gap: var(--shuttle-space-3);
    padding: var(--shuttle-space-2) var(--shuttle-space-3);
    background: color-mix(in oklab, var(--shuttle-danger) 10%, transparent);
    color: var(--shuttle-danger);
    border: 1px solid color-mix(in oklab, var(--shuttle-danger) 30%, transparent);
    border-radius: var(--shuttle-radius-md);
    font-size: var(--shuttle-text-sm);
  }
  .msg { display: flex; align-items: center; gap: var(--shuttle-space-2); }
  .retry {
    margin-left: auto;
    background: transparent; border: 1px solid var(--shuttle-danger); color: var(--shuttle-danger);
    padding: 2px var(--shuttle-space-2); border-radius: var(--shuttle-radius-sm);
    font-size: var(--shuttle-text-xs); cursor: pointer;
  }
</style>
