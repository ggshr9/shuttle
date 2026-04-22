<script lang="ts">
  import { Icon } from '@/ui'
  type State = 'disconnected' | 'connecting' | 'connected'
  interface Labels {
    connect: string      // announced when disconnected
    disconnect: string   // announced when connected
    connecting: string   // announced while transitioning
  }
  interface Props {
    state: State
    labels?: Labels      // keep English defaults; parent passes localized copy
    onToggle?: () => void
  }
  let {
    state,
    labels = { connect: 'Connect', disconnect: 'Disconnect', connecting: 'Connecting' },
    onToggle,
  }: Props = $props()
  const isConnected = $derived(state === 'connected')
  const isConnecting = $derived(state === 'connecting')
  const label = $derived(
    isConnecting ? labels.connecting : isConnected ? labels.disconnect : labels.connect
  )

  function handleClick() {
    if (isConnecting) return
    // Haptic on touch devices (no-op on desktop browsers without vibrate).
    if (typeof navigator !== 'undefined' && 'vibrate' in navigator) {
      navigator.vibrate?.(10)
    }
    onToggle?.()
  }
</script>

<button
  class="power"
  data-state={state}
  role="switch"
  aria-checked={isConnected}
  aria-label={label}
  aria-busy={isConnecting}
  disabled={isConnecting}
  onclick={handleClick}
>
  {#if isConnecting}
    <span class="spinner" aria-hidden="true"></span>
  {/if}
  <Icon name="power" size={44} />
</button>

<style>
  .power {
    width: 120px; height: 120px;
    border-radius: 50%;
    border: 2px solid var(--shuttle-border);
    background: var(--shuttle-bg-subtle);
    color: var(--shuttle-fg-muted);
    display: flex; align-items: center; justify-content: center;
    cursor: pointer;
    position: relative;
    transition: background var(--shuttle-duration), border-color var(--shuttle-duration), color var(--shuttle-duration);
  }
  .power[data-state="connecting"] {
    border-color: var(--shuttle-warning, #d29922);
    color: var(--shuttle-warning, #d29922);
    background: color-mix(in srgb, var(--shuttle-warning, #d29922) 12%, transparent);
  }
  .power[data-state="connected"] {
    border-color: var(--shuttle-success, #3fb950);
    color: var(--shuttle-success, #3fb950);
    background: color-mix(in srgb, var(--shuttle-success, #3fb950) 12%, transparent);
  }
  .power:disabled { cursor: wait; }
  .power:focus-visible { outline: 2px solid var(--shuttle-accent); outline-offset: 3px; }

  .spinner {
    position: absolute; inset: -6px;
    border-radius: 50%;
    border: 2px dashed currentColor;
    opacity: 0.6;
    animation: spin 2s linear infinite;
  }
  @keyframes spin {
    from { transform: rotate(0deg); }
    to   { transform: rotate(360deg); }
  }
</style>
