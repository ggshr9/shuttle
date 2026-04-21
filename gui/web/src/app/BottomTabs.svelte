<script lang="ts">
  import { Link, useRoute } from '@/lib/router'
  import { Icon } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { primaryNav } from './nav'

  const route = useRoute()
  const items = primaryNav()

  function isActive(path: string): boolean {
    if (path === '/') return route.path === '/'
    return route.path === path || route.path.startsWith(path + '/')
  }
</script>

<nav class="tabs" aria-label="Primary navigation" role="tablist">
  {#each items as item}
    <Link
      to={item.path}
      class={'tab ' + (isActive(item.path) ? 'active' : '')}
      role="tab"
      aria-selected={isActive(item.path)}
    >
      <span class="icon"><Icon name={item.icon} size={20} /></span>
      <span class="label">{t(item.label)}</span>
    </Link>
  {/each}
</nav>

<style>
  .tabs {
    display: flex;
    height: 56px;
    padding-bottom: env(safe-area-inset-bottom);
    border-top: 1px solid var(--shuttle-border);
    background: var(--shuttle-bg-base);
    position: sticky; bottom: 0;
  }
  :global(a.tab) {
    flex: 1;
    display: flex; flex-direction: column;
    align-items: center; justify-content: center;
    gap: 2px;
    text-decoration: none;
    color: var(--shuttle-fg-muted);
    font-size: 10px;
    min-height: 44px;
    transition: color var(--shuttle-duration);
  }
  :global(a.tab.active) { color: var(--shuttle-accent); }
  .icon { width: 20px; height: 20px; display: inline-flex; }
  .label { font-weight: var(--shuttle-weight-medium); letter-spacing: 0.02em; }
</style>
