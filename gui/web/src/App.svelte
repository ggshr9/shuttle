<script lang="ts">
  import { onMount } from 'svelte'
  import { t, subscribeLocale } from './lib/i18n/index'
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
    return subscribeLocale((newLocale) => {
      locale = newLocale
    })
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
  <nav class="tabs">
    {#each tabs as item}
      <button
        class:active={tab === item.id}
        onclick={() => (tab = item.id)}
      >
        {item.label}
      </button>
    {/each}
  </nav>

  <main>
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
  :global(body) {
    margin: 0;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    background: #0f1117;
    color: #e1e4e8;
  }

  .app {
    max-width: 900px;
    margin: 0 auto;
    padding: 16px;
  }

  .tabs {
    display: flex;
    gap: 4px;
    border-bottom: 1px solid #2d333b;
    margin-bottom: 20px;
  }

  .tabs button {
    background: none;
    border: none;
    color: #8b949e;
    padding: 10px 16px;
    cursor: pointer;
    font-size: 14px;
    border-bottom: 2px solid transparent;
    transition: color 0.2s, border-color 0.2s;
  }

  .tabs button:hover {
    color: #e1e4e8;
  }

  .tabs button.active {
    color: #58a6ff;
    border-bottom-color: #58a6ff;
  }

  .loading {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    height: 100vh;
    color: #8b949e;
    font-size: 14px;
    gap: 16px;
  }

  .spinner {
    width: 32px;
    height: 32px;
    border: 3px solid #2d333b;
    border-top-color: #58a6ff;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .api-error {
    background: rgba(248, 81, 73, 0.1);
    border: 1px solid #f85149;
    color: #f85149;
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
    background: #f85149;
    color: #fff;
    border: none;
    border-radius: 4px;
    padding: 4px 12px;
    cursor: pointer;
    font-size: 12px;
  }
</style>
