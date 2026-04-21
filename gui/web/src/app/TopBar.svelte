<script lang="ts">
  import { Icon } from '@/ui'
  import { navigate } from '@/lib/router'

  interface Props {
    title: string
    showMenuButton?: boolean
    onMenuClick?: () => void
  }
  let { title, showMenuButton = false, onMenuClick }: Props = $props()
</script>

<header class="topbar">
  {#if showMenuButton}
    <button class="icon-btn" aria-label="Menu" onclick={onMenuClick}>
      <Icon name="menu" size={18} />
    </button>
  {:else}
    <div class="brand">
      <div class="logo">S</div>
    </div>
  {/if}
  <h1 class="title">{title}</h1>
  <div class="spacer"></div>
  <button
    class="icon-btn"
    aria-label="Settings"
    onclick={() => navigate('/settings')}
  >
    <Icon name="settings" size={18} />
  </button>
</header>

<style>
  .topbar {
    display: flex; align-items: center; gap: var(--shuttle-space-3);
    height: 48px;
    padding: 0 var(--shuttle-space-3);
    padding-top: env(safe-area-inset-top);
    border-bottom: 1px solid var(--shuttle-border);
    background: var(--shuttle-bg-base);
    position: sticky; top: 0; z-index: 10;
  }
  .brand { display: flex; }
  .logo {
    width: 22px; height: 22px;
    background: var(--shuttle-accent); color: var(--shuttle-accent-fg);
    border-radius: var(--shuttle-radius-sm);
    display: flex; align-items: center; justify-content: center;
    font-weight: 600; font-size: 11px;
  }
  .title {
    font-size: var(--shuttle-text-base);
    font-weight: var(--shuttle-weight-semibold);
    margin: 0;
    color: var(--shuttle-fg-primary);
  }
  .spacer { flex: 1; }
  .icon-btn {
    background: transparent; border: 0; cursor: pointer;
    padding: var(--shuttle-space-1);
    color: var(--shuttle-fg-secondary);
    border-radius: var(--shuttle-radius-sm);
    min-height: 44px; min-width: 44px;
    display: flex; align-items: center; justify-content: center;
  }
  .icon-btn:hover { color: var(--shuttle-fg-primary); }
</style>
