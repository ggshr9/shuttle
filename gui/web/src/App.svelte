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
  let sidebarCollapsed = $state(false)

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
    { id: 'dashboard', label: t('nav.dashboard'), icon: 'dashboard' },
    { id: 'servers', label: t('nav.servers'), icon: 'servers' },
    { id: 'subscriptions', label: t('nav.subscriptions'), icon: 'subscriptions' },
    { id: 'routing', label: t('nav.routing'), icon: 'routing' },
    { id: 'logs', label: t('nav.logs'), icon: 'logs' },
    { id: 'settings', label: t('nav.settings'), icon: 'settings' },
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
    <p>{t('app.connecting')}</p>
  </div>
{/if}

{#if initialized}
{#if apiError}
  <div class="api-error">
    {t('app.backendError')}
    <button onclick={checkFirstRun}>{t('app.retry')}</button>
  </div>
{/if}
<div class="app" class:collapsed={sidebarCollapsed}>
  <nav class="sidebar" role="tablist" aria-label="Navigation" onkeydown={handleTabKeydown}>
    <div class="sidebar-header">
      <div class="logo">
        <svg width="28" height="28" viewBox="0 0 28 28" fill="none">
          <rect width="28" height="28" rx="8" fill="var(--accent)"/>
          <path d="M8 14l4-6 4 6-4 6-4-6zm4-2l4 6h-8l4-6z" fill="white" opacity="0.9"/>
          <path d="M14 10l4 4-4 4" stroke="white" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
        {#if !sidebarCollapsed}
          <span class="logo-text">Shuttle</span>
        {/if}
      </div>
      <button class="collapse-btn" onclick={() => sidebarCollapsed = !sidebarCollapsed} aria-label="Toggle sidebar">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
          {#if sidebarCollapsed}
            <path d="M6 3l5 5-5 5" stroke="currentColor" stroke-width="1.5" fill="none" stroke-linecap="round"/>
          {:else}
            <path d="M10 3l-5 5 5 5" stroke="currentColor" stroke-width="1.5" fill="none" stroke-linecap="round"/>
          {/if}
        </svg>
      </button>
    </div>

    <div class="nav-items">
      {#each tabs as item, i}
        <button
          role="tab"
          aria-selected={tab === item.id}
          aria-controls="tabpanel"
          id={'tab-' + item.id}
          tabindex={tab === item.id ? 0 : -1}
          class="nav-item"
          class:active={tab === item.id}
          onclick={() => (tab = item.id)}
        >
          <span class="nav-icon">
            {#if item.icon === 'dashboard'}
              <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
                <rect x="3" y="3" width="6" height="6" rx="1"/>
                <rect x="11" y="3" width="6" height="6" rx="1"/>
                <rect x="3" y="11" width="6" height="6" rx="1"/>
                <rect x="11" y="11" width="6" height="6" rx="1"/>
              </svg>
            {:else if item.icon === 'servers'}
              <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
                <rect x="3" y="3" width="14" height="5" rx="1.5"/>
                <rect x="3" y="12" width="14" height="5" rx="1.5"/>
                <circle cx="6" cy="5.5" r="1" fill="currentColor"/>
                <circle cx="6" cy="14.5" r="1" fill="currentColor"/>
              </svg>
            {:else if item.icon === 'subscriptions'}
              <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
                <path d="M4 5h12M4 10h12M4 15h8"/>
                <circle cx="16" cy="15" r="2"/>
              </svg>
            {:else if item.icon === 'routing'}
              <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
                <circle cx="5" cy="10" r="2"/>
                <circle cx="15" cy="5" r="2"/>
                <circle cx="15" cy="15" r="2"/>
                <path d="M7 10h3l2-5h1M10 10l2 5h1"/>
              </svg>
            {:else if item.icon === 'logs'}
              <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
                <path d="M5 4h10a1 1 0 011 1v10a1 1 0 01-1 1H5a1 1 0 01-1-1V5a1 1 0 011-1z"/>
                <path d="M7 8h6M7 11h4"/>
              </svg>
            {:else if item.icon === 'settings'}
              <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
                <circle cx="10" cy="10" r="3"/>
                <path d="M10 3v2M10 15v2M3 10h2M15 10h2M5.05 5.05l1.41 1.41M13.54 13.54l1.41 1.41M5.05 14.95l1.41-1.41M13.54 6.46l1.41-1.41"/>
              </svg>
            {/if}
          </span>
          {#if !sidebarCollapsed}
            <span class="nav-label">{item.label}</span>
          {/if}
        </button>
      {/each}
    </div>
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
  /* ========== Tailscale-inspired Design Tokens ========== */
  :global(:root),
  :global([data-theme="dark"]) {
    --bg-primary: #0c0c14;
    --bg-secondary: #14141f;
    --bg-tertiary: #1e1e2e;
    --bg-surface: #181825;
    --bg-hover: #232336;
    --bg-sidebar: #111119;
    --text-primary: #f0f0f5;
    --text-secondary: #9394a5;
    --text-muted: #55566a;
    --border: #2a2a3d;
    --border-light: #353549;
    --accent: #4f6df5;
    --accent-hover: #6381ff;
    --accent-subtle: rgba(79, 109, 245, 0.12);
    --accent-green: #34d399;
    --accent-green-subtle: rgba(52, 211, 153, 0.12);
    --accent-purple: #a78bfa;
    --accent-red: #f87171;
    --accent-red-subtle: rgba(248, 113, 113, 0.1);
    --accent-yellow: #fbbf24;
    --accent-yellow-subtle: rgba(251, 191, 36, 0.1);
    --btn-bg: #4f6df5;
    --btn-bg-hover: #6381ff;
    --overlay-bg: rgba(0, 0, 0, 0.6);
    --shadow-sm: 0 1px 2px rgba(0, 0, 0, 0.3);
    --shadow-md: 0 4px 12px rgba(0, 0, 0, 0.4);
    --shadow-lg: 0 8px 32px rgba(0, 0, 0, 0.5);
    --radius-sm: 6px;
    --radius-md: 10px;
    --radius-lg: 14px;
    --radius-xl: 20px;
    --sidebar-width: 220px;
    --sidebar-collapsed: 64px;
  }

  :global([data-theme="light"]) {
    --bg-primary: #f8f9fc;
    --bg-secondary: #ffffff;
    --bg-tertiary: #eef0f5;
    --bg-surface: #f3f4f8;
    --bg-hover: #e8eaf0;
    --bg-sidebar: #ffffff;
    --text-primary: #111827;
    --text-secondary: #6b7280;
    --text-muted: #9ca3af;
    --border: #e2e4eb;
    --border-light: #eef0f5;
    --accent: #4f6df5;
    --accent-hover: #3b57e0;
    --accent-subtle: rgba(79, 109, 245, 0.08);
    --accent-green: #10b981;
    --accent-green-subtle: rgba(16, 185, 129, 0.08);
    --accent-purple: #8b5cf6;
    --accent-red: #ef4444;
    --accent-red-subtle: rgba(239, 68, 68, 0.08);
    --accent-yellow: #f59e0b;
    --accent-yellow-subtle: rgba(245, 158, 11, 0.08);
    --btn-bg: #4f6df5;
    --btn-bg-hover: #3b57e0;
    --overlay-bg: rgba(0, 0, 0, 0.3);
    --shadow-sm: 0 1px 2px rgba(0, 0, 0, 0.06);
    --shadow-md: 0 4px 12px rgba(0, 0, 0, 0.08);
    --shadow-lg: 0 8px 32px rgba(0, 0, 0, 0.12);
  }

  :global(body) {
    margin: 0;
    font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    background: var(--bg-primary);
    color: var(--text-primary);
    -webkit-font-smoothing: antialiased;
    -moz-osx-font-smoothing: grayscale;
  }

  :global(*) {
    box-sizing: border-box;
  }

  :global(:focus-visible) {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }

  /* Scrollbar styling */
  :global(::-webkit-scrollbar) {
    width: 6px;
  }
  :global(::-webkit-scrollbar-track) {
    background: transparent;
  }
  :global(::-webkit-scrollbar-thumb) {
    background: var(--border);
    border-radius: 3px;
  }
  :global(::-webkit-scrollbar-thumb:hover) {
    background: var(--text-muted);
  }

  /* ========== Layout ========== */
  .app {
    display: flex;
    height: 100vh;
    overflow: hidden;
  }

  .sidebar {
    width: var(--sidebar-width);
    min-width: var(--sidebar-width);
    background: var(--bg-sidebar);
    border-right: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    transition: width 0.2s ease, min-width 0.2s ease;
    overflow: hidden;
  }

  .app.collapsed .sidebar {
    width: var(--sidebar-collapsed);
    min-width: var(--sidebar-collapsed);
  }

  .sidebar-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 20px 16px 16px;
    min-height: 60px;
  }

  .logo {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .logo-text {
    font-size: 18px;
    font-weight: 700;
    color: var(--text-primary);
    letter-spacing: -0.02em;
    white-space: nowrap;
  }

  .collapse-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 4px;
    border-radius: var(--radius-sm);
    display: flex;
    align-items: center;
    justify-content: center;
    transition: color 0.15s, background 0.15s;
  }

  .collapse-btn:hover {
    color: var(--text-primary);
    background: var(--bg-hover);
  }

  .app.collapsed .collapse-btn {
    margin: 0 auto;
  }

  .nav-items {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 0 8px;
    flex: 1;
  }

  .nav-item {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 12px;
    border: none;
    background: transparent;
    color: var(--text-secondary);
    cursor: pointer;
    border-radius: var(--radius-md);
    font-size: 14px;
    font-weight: 500;
    transition: all 0.15s ease;
    white-space: nowrap;
    text-align: left;
    width: 100%;
  }

  .nav-item:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
  }

  .nav-item.active {
    background: var(--accent-subtle);
    color: var(--accent);
  }

  .nav-item.active .nav-icon {
    color: var(--accent);
  }

  .nav-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    flex-shrink: 0;
  }

  .nav-label {
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .app.collapsed .nav-item {
    justify-content: center;
    padding: 10px;
  }

  main {
    flex: 1;
    overflow-y: auto;
    padding: 28px 32px;
  }

  /* ========== States ========== */
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
    background: var(--accent-red-subtle);
    border: 1px solid var(--accent-red);
    color: var(--accent-red);
    padding: 8px 16px;
    border-radius: var(--radius-md);
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
    border-radius: var(--radius-sm);
    padding: 4px 12px;
    cursor: pointer;
    font-size: 12px;
  }
</style>
