<script lang="ts">
  import { onMount, type Component } from 'svelte'
  import { getConfig } from '@/lib/api/endpoints'
  import { t } from '@/lib/i18n/index'
  import Shell from './Shell.svelte'
  import Toaster from './Toaster.svelte'

  let Onboarding = $state<Component<{ onComplete: () => void }> | null>(null)

  let initialized = $state(false)
  let showOnboarding = $state(false)
  let apiError = $state(false)

  async function checkFirstRun() {
    try {
      const cfg = await getConfig()
      const hasServers = !!(cfg.server?.addr || (cfg.servers && cfg.servers.length > 0))
      showOnboarding = !hasServers
      apiError = false
      if (showOnboarding) {
        const mod = await import('@/features/onboarding')
        Onboarding = mod.Onboarding as Component<{ onComplete: () => void }>
      }
    } catch {
      showOnboarding = false
      apiError = true
    }
    initialized = true
  }

  onMount(() => { checkFirstRun() })

  function handleOnboardingComplete() {
    showOnboarding = false
  }
</script>

<Toaster />

{#if !initialized}
  <div class="center">
    <div class="spin" aria-label="Loading"></div>
  </div>
{:else if showOnboarding && Onboarding}
  <Onboarding onComplete={handleOnboardingComplete} />
{:else}
  {#if apiError}
    <div class="api-error">
      <span>{t('app.backendUnavailable')}</span>
      <button onclick={() => location.reload()}>{t('app.retry')}</button>
    </div>
  {/if}
  <Shell />
{/if}

<style>
  .center {
    display: flex; align-items: center; justify-content: center;
    min-height: 100vh;
    background: var(--shuttle-bg-base);
  }
  .spin {
    width: 28px; height: 28px;
    border: 3px solid var(--shuttle-border);
    border-top-color: var(--shuttle-fg-primary);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }
  @keyframes spin { to { transform: rotate(360deg); } }

  .api-error {
    background: color-mix(in oklab, var(--shuttle-danger) 10%, transparent);
    color: var(--shuttle-danger);
    border: 1px solid color-mix(in oklab, var(--shuttle-danger) 30%, transparent);
    padding: var(--shuttle-space-2) var(--shuttle-space-3);
    font-size: var(--shuttle-text-sm);
    display: flex; justify-content: space-between; align-items: center;
    font-family: var(--shuttle-font-sans);
  }
  .api-error button {
    background: transparent; border: 1px solid var(--shuttle-danger);
    color: var(--shuttle-danger); padding: 2px 8px;
    border-radius: var(--shuttle-radius-sm); cursor: pointer; font-size: var(--shuttle-text-xs);
  }
</style>
