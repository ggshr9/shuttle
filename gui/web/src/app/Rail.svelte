<script lang="ts">
  import { Link, useRoute } from '@/lib/router'
  import { Icon } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { nav } from './nav'

  const route = useRoute()
  function isActive(path: string): boolean {
    if (path === '/') return route.path === '/'
    return route.path === path || route.path.startsWith(path + '/')
  }
</script>

<aside class="rail" aria-label="Primary navigation">
  {#each nav as item}
    <Link
      to={item.path}
      class={'rail-item ' + (isActive(item.path) ? 'active' : '')}
      aria-label={t(item.label)}
    >
      <span class="icon"><Icon name={item.icon} size={20} /></span>
      <span class="mini-label">{t(item.label)}</span>
    </Link>
  {/each}
</aside>

<style>
  .rail {
    width: 64px; min-width: 64px;
    display: flex; flex-direction: column; gap: 2px;
    padding: var(--shuttle-space-3) var(--shuttle-space-1);
    border-right: 1px solid var(--shuttle-border);
    background: var(--shuttle-bg-base);
  }
  :global(a.rail-item) {
    display: flex; flex-direction: column; align-items: center; gap: 2px;
    padding: var(--shuttle-space-2) var(--shuttle-space-1);
    border-radius: var(--shuttle-radius-sm);
    text-decoration: none;
    color: var(--shuttle-fg-muted);
    font-size: 9px;
    min-height: 44px;
  }
  :global(a.rail-item.active),
  :global(a.rail-item:hover) {
    color: var(--shuttle-fg-primary);
    background: var(--shuttle-bg-subtle);
  }
  .icon { width: 20px; height: 20px; display: inline-flex; }
  .mini-label { letter-spacing: 0.02em; }
</style>
