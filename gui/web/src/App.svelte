<script lang="ts">
  import { onMount } from 'svelte'
  import { t, subscribeLocale } from './lib/i18n/index'
  import { subscribeTheme } from './lib/theme'
  import { api } from './lib/api'
  import Onboarding from './lib/Onboarding.svelte'
  import Toast from './lib/Toast.svelte'

  let tab = $state('dashboard')
  let locale = $state('en')
  let showOnboarding = $state(false)
  let initialized = $state(false)

  let apiError = $state(false)

  // Check if user needs onboarding (no servers configured)
  async function checkFirstRun() {
    try {
      const cfg = await api.getConfig()
      const hasServers = cfg.server?.addr || (cfg.servers && cfg.servers.length > 0)
      showOnboarding = !hasServers
      apiError = false
    } catch {
      // If we can't load config, don't show onboarding but still show UI
      showOnboarding = false
      apiError = true
    }
    initialized = true
  }

  function handleOnboardingComplete() {
    showOnboarding = false
  }

  // Subscribe to locale changes for reactivity
  onMount(() => {
    checkFirstRun()
    const unsubLocale = subscribeLocale((newLocale) => {
      locale = newLocale
    })
    const unsubTheme = subscribeTheme(() => {})
    return () => { unsubLocale(); unsubTheme() }
  })

  // Reactive tabs that update when locale changes
  const tabs = $derived([
    { id: 'dashboard', label: t('nav.dashboard') },
    { id: 'servers', label: t('nav.servers') },
    { id: 'subscriptions', label: t('nav.subscriptions') },
    { id: 'routing', label: t('nav.routing') },
    { id: 'logs', label: t('nav.logs') },
    { id: 'settings', label: t('nav.settings') },
  ])

  // Force dependency on locale for reactivity
  $effect(() => { void locale })

  function handleTabKeydown(e: KeyboardEvent) {
    const tabIds = tabs.map(t => t.id)
    const idx = tabIds.indexOf(tab)
    let next = -1
    if (e.key === 'ArrowRight' || e.key === 'ArrowDown') {
      next = (idx + 1) % tabIds.length
    } else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') {
      next = (idx - 1 + tabIds.length) % tabIds.length
    } else if (e.key === 'Home') {
      next = 0
    } else if (e.key === 'End') {
      next = tabIds.length - 1
    }
    if (next >= 0) {
      e.preventDefault()
      tab = tabIds[next]
      const el = document.getElementById('tab-' + tab)
      el?.focus()
    }
  }
</script>

<Toast />

{#if showOnboarding}
  <Onboarding onComplete={handleOnboardingComplete} />
{/if}

{#if !initialized}
  <div class="loading">
    <div class="spinner"></div>
    <p>Connecting to Shuttle...</p>
  </div>
{/if}

{#if initialized}
{#if apiError}
  <div class="api-error">
    Backend unreachable. Check that Shuttle is running.
    <button onclick={checkFirstRun}>Retry</button>
  </div>
{/if}
<div class="app">
  <nav class="tabs" role="tablist" aria-label="Navigation" onkeydown={handleTabKeydown}>
    {#each tabs as item, i}
      <button
        role="tab"
        aria-selected={tab === item.id}
        aria-controls="tabpanel"
        id={'tab-' + item.id}
        tabindex={tab === item.id ? 0 : -1}
        class:active={tab === item.id}
        onclick={() => (tab = item.id)}
      >
        {item.label}
      </button>
    {/each}
  </nav>

  <main role="tabpanel" id="tabpanel" aria-labelledby={'tab-' + tab} tabindex="0">
    {#if tab === 'dashboard'}
      {#await import('./pages/Dashboard.svelte') then { default: Dashboard }}
        <Dashboard />
      {/await}
    {:else if tab === 'servers'}
      {#await import('./pages/Servers.svelte') then { default: Servers }}
        <Servers />
      {/await}
    {:else if tab === 'subscriptions'}
      {#await import('./pages/Subscriptions.svelte') then { default: Subscriptions }}
        <Subscriptions />
      {/await}
    {:else if tab === 'routing'}
      {#await import('./pages/Routing.svelte') then { default: Routing }}
        <Routing />
      {/await}
    {:else if tab === 'logs'}
      {#await import('./pages/Logs.svelte') then { default: Logs }}
        <Logs />
      {/await}
    {:else if tab === 'settings'}
      {#await import('./pages/Settings.svelte') then { default: Settings }}
        <Settings />
      {/await}
    {/if}
  </main>
</div>
{/if}

<style>
  :global(:root),
  :global([data-theme="dark"]) {
    --bg-primary: #0f1117;
    --bg-secondary: #161b22;
    --bg-tertiary: #21262d;
    --bg-surface: #0d1117;
    --text-primary: #e1e4e8;
    --text-secondary: #8b949e;
    --text-muted: #484f58;
    --border: #2d333b;
    --accent: #58a6ff;
    --accent-green: #3fb950;
    --accent-purple: #a371f7;
    --accent-red: #f85149;
    --btn-bg: #238636;
    --btn-bg-hover: #2ea043;
    --overlay-bg: rgba(0, 0, 0, 0.6);
  }

  :global([data-theme="light"]) {
    --bg-primary: #ffffff;
    --bg-secondary: #f6f8fa;
    --bg-tertiary: #e1e4e8;
    --bg-surface: #f0f3f6;
    --text-primary: #1f2328;
    --text-secondary: #656d76;
    --text-muted: #8b949e;
    --border: #d0d7de;
    --accent: #0969da;
    --accent-green: #1a7f37;
    --accent-purple: #8250df;
    --accent-red: #cf222e;
    --btn-bg: #1a7f37;
    --btn-bg-hover: #218a3c;
    --overlay-bg: rgba(0, 0, 0, 0.3);
  }

  :global(body) {
    margin: 0;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    background: var(--bg-primary);
    color: var(--text-primary);
  }

  .app {
    max-width: 900px;
    margin: 0 auto;
    padding: 16px;
  }

  .tabs {
    display: flex;
    gap: 4px;
    border-bottom: 1px solid var(--border);
    margin-bottom: 20px;
  }

  .tabs button {
    background: none;
    border: none;
    color: var(--text-secondary);
    padding: 10px 16px;
    cursor: pointer;
    font-size: 14px;
    border-bottom: 2px solid transparent;
    transition: color 0.2s, border-color 0.2s;
  }

  .tabs button:hover {
    color: var(--text-primary);
  }

  .tabs button.active {
    color: var(--accent);
    border-bottom-color: var(--accent);
  }

  .tabs button:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: -2px;
    border-radius: 4px;
  }

  :global(:focus-visible) {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }

  .loading {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    height: 100vh;
    color: var(--text-secondary);
    font-size: 14px;
    gap: 16px;
  }

  .spinner {
    width: 32px;
    height: 32px;
    border: 3px solid var(--border);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .api-error {
    background: rgba(248, 81, 73, 0.1);
    border: 1px solid var(--accent-red);
    color: var(--accent-red);
    padding: 8px 16px;
    border-radius: 6px;
    max-width: 900px;
    margin: 8px auto;
    font-size: 13px;
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .api-error button {
    background: var(--accent-red);
    color: #fff;
    border: none;
    border-radius: 4px;
    padding: 4px 12px;
    cursor: pointer;
    font-size: 12px;
  }
</style>
