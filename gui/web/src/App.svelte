<script>
  import { onMount } from 'svelte'
  import { t, subscribeLocale } from './lib/i18n/index.js'

  let tab = $state('dashboard')
  let locale = $state('en')

  // Subscribe to locale changes for reactivity
  onMount(() => {
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
</style>
