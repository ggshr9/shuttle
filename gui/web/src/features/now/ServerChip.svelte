<script lang="ts">
  type State = 'disconnected' | 'connecting' | 'connected'
  interface Props {
    serverName: string
    transport: string
    state: State
    onClick?: () => void
  }
  let { serverName, transport, state, onClick }: Props = $props()
</script>

<button class="chip" data-state={state} onclick={() => onClick?.()} aria-label="Change server">
  <span class="dot" aria-hidden="true"></span>
  <span class="text">
    {serverName}
    {#if transport}<span class="transport" aria-hidden="true">· {transport}</span>{/if}
  </span>
  <span class="caret" aria-hidden="true">▾</span>
</button>

<style>
  .chip {
    display: inline-flex; align-items: center; gap: var(--shuttle-space-2);
    padding: var(--shuttle-space-2) var(--shuttle-space-3);
    border: 1px solid var(--shuttle-border);
    border-radius: 999px;
    background: transparent;
    color: var(--shuttle-fg-secondary);
    font-size: var(--shuttle-text-sm);
    cursor: pointer;
    min-height: 44px;
  }
  .chip[data-state="connected"] .dot { background: var(--shuttle-success, #3fb950); }
  .chip[data-state="connecting"] .dot { background: var(--shuttle-warning, #d29922); }
  .dot {
    width: 6px; height: 6px; border-radius: 50%;
    background: var(--shuttle-fg-muted);
  }
  .transport { color: var(--shuttle-fg-muted); margin-left: var(--shuttle-space-1); }
  .caret { color: var(--shuttle-fg-muted); font-size: 10px; }
</style>
