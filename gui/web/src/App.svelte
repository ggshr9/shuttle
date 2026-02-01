<script>
  let tab = $state('dashboard')

  const tabs = [
    { id: 'dashboard', label: 'Dashboard' },
    { id: 'servers', label: 'Servers' },
    { id: 'routing', label: 'Routing' },
    { id: 'logs', label: 'Logs' },
    { id: 'settings', label: 'Settings' },
  ]
</script>

<div class="app">
  <nav class="tabs">
    {#each tabs as t}
      <button
        class:active={tab === t.id}
        onclick={() => (tab = t.id)}
      >
        {t.label}
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
