<script lang="ts">
  import { Router } from '@/lib/router'
  import { viewport } from '@/lib/viewport.svelte'
  import Sidebar from './Sidebar.svelte'
  import Rail from './Rail.svelte'
  import BottomTabs from './BottomTabs.svelte'
  import TopBar from './TopBar.svelte'
  import { nav } from './nav'
  import { useRoute } from '@/lib/router'
  import { t } from '@/lib/i18n/index'
  import { routes } from './routes'

  const route = useRoute()

  const currentTitle = $derived.by(() => {
    const item = nav.find(n => n.path === route.path || route.path.startsWith(n.path + '/'))
    return item ? t(item.label) : ''
  })

  $effect(() => {
    document.body.dataset.touch = viewport.isTouch ? '1' : '0'
  })
</script>

<div class="shell" data-form={viewport.form}>
  {#if viewport.isMobile}
    <TopBar title={currentTitle} />
    <main>
      <Router {routes} />
    </main>
    <BottomTabs />
  {:else if viewport.isTablet}
    <Rail />
    <main>
      <Router {routes} />
    </main>
  {:else}
    <Sidebar />
    <main>
      <Router {routes} />
    </main>
  {/if}
</div>

<style>
  .shell {
    display: flex;
    min-height: 100vh;
    background: var(--shuttle-bg-base);
  }
  .shell[data-form="xs"],
  .shell[data-form="sm"] {
    flex-direction: column;
  }
  main {
    flex: 1; min-width: 0;
    overflow-y: auto;
    overscroll-behavior: contain;
    padding: var(--shuttle-space-5) var(--shuttle-space-6);
  }
  .shell[data-form="xs"] main,
  .shell[data-form="sm"] main {
    padding: var(--shuttle-space-3);
  }
  :global([data-touch="1"] button),
  :global([data-touch="1"] a) {
    min-height: 44px;
  }
  @media (hover: none) {
    /* touch devices: suppress hover states via pointer-events or JS-driven classes */
  }
</style>
