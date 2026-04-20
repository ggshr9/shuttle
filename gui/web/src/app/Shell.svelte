<script lang="ts">
  import { Router } from '@/lib/router'
  import Sidebar from './Sidebar.svelte'
  import { routes } from './routes'

  let collapsed = $state(
    typeof localStorage !== 'undefined' && localStorage.getItem('shuttle-sidebar-collapsed') === '1'
  )

  function toggleCollapsed() {
    collapsed = !collapsed
    try {
      localStorage.setItem('shuttle-sidebar-collapsed', collapsed ? '1' : '0')
    } catch {
      // ignore
    }
  }
</script>

<div class="shell">
  <Sidebar {routes} {collapsed} onToggleCollapsed={toggleCollapsed} />
  <main>
    <Router {routes} />
  </main>
</div>

<style>
  .shell {
    display: flex;
    min-height: 100vh;
    background: var(--shuttle-bg-base);
  }
  main {
    flex: 1;
    overflow-y: auto;
    padding: var(--shuttle-space-5) var(--shuttle-space-6);
  }
</style>
