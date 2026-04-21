<script lang="ts">
  import { Link, useRoute } from '@/lib/router'
  import { Icon, Button } from '@/ui'
  import { theme } from '@/lib/theme.svelte'
  import { t } from '@/lib/i18n/index'
  import { navBySection } from './nav'
  import type { IconName } from '@/app/icons'

  interface Props {
    collapsed?: boolean
    onToggleCollapsed?: () => void
  }

  let { collapsed = false, onToggleCollapsed }: Props = $props()
  const route = useRoute()

  const sections = {
    overview: navBySection('overview'),
    network:  navBySection('network'),
    system:   navBySection('system'),
  }

  function isActive(path: string): boolean {
    if (path === '/') return route.path === '/'
    return route.path === path || route.path.startsWith(path + '/')
  }
</script>

<aside class="sidebar" class:collapsed>
  <div class="brand">
    <div class="logo">S</div>
    {#if !collapsed}<span class="name">Shuttle</span>{/if}
  </div>

  {#snippet section(heading: string, items: ReturnType<typeof navBySection>)}
    {#if items.length > 0}
      {#if !collapsed}<div class="heading">{heading}</div>{/if}
      <nav>
        {#each items as item}
          <Link to={item.path} class={'item ' + (isActive(item.path) ? 'on' : '')}>
            <span class="ico"><Icon name={item.icon as IconName} size={16} /></span>
            {#if !collapsed}<span>{t(item.label)}</span>{/if}
          </Link>
        {/each}
      </nav>
    {/if}
  {/snippet}

  {@render section(t('nav.section.overview'), sections.overview)}
  {@render section(t('nav.section.network'), sections.network)}
  {@render section(t('nav.section.system'), sections.system)}

  <div class="footer">
    <Button size="sm" variant="ghost" onclick={() => theme.toggle()}>
      {#if !collapsed}{theme.current}{:else}·{/if}
    </Button>
    {#if onToggleCollapsed}
      <Button size="sm" variant="ghost" onclick={onToggleCollapsed}>
        <Icon name={collapsed ? 'chevronRight' : 'chevronLeft'} size={14} />
      </Button>
    {/if}
  </div>
</aside>

<style>
  .sidebar {
    width: 220px;
    min-width: 220px;
    background: var(--shuttle-bg-base);
    border-right: 1px solid var(--shuttle-border);
    display: flex;
    flex-direction: column;
    padding: var(--shuttle-space-4) var(--shuttle-space-2);
    transition: width var(--shuttle-duration) var(--shuttle-easing);
    font-family: var(--shuttle-font-sans);
  }
  .sidebar.collapsed { width: 60px; min-width: 60px; }

  .brand {
    display: flex; align-items: center; gap: var(--shuttle-space-2);
    padding: var(--shuttle-space-1) var(--shuttle-space-2) var(--shuttle-space-4);
  }
  .logo {
    width: 22px; height: 22px;
    background: var(--shuttle-accent); color: var(--shuttle-accent-fg);
    border-radius: var(--shuttle-radius-sm);
    display: flex; align-items: center; justify-content: center;
    font-weight: var(--shuttle-weight-semibold); font-size: 11px;
  }
  .name {
    font-size: var(--shuttle-text-base);
    font-weight: var(--shuttle-weight-semibold);
    letter-spacing: var(--shuttle-tracking-tight);
    color: var(--shuttle-fg-primary);
  }

  .heading {
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-muted);
    text-transform: uppercase;
    letter-spacing: 0.08em;
    padding: var(--shuttle-space-4) var(--shuttle-space-2) var(--shuttle-space-1);
  }

  nav { display: flex; flex-direction: column; gap: 1px; }

  :global(a.item) {
    display: flex; align-items: center; gap: var(--shuttle-space-3);
    padding: var(--shuttle-space-1) var(--shuttle-space-2);
    height: 30px;
    border-radius: var(--shuttle-radius-sm);
    color: var(--shuttle-fg-secondary);
    font-size: var(--shuttle-text-sm);
    text-decoration: none;
    transition: background var(--shuttle-duration), color var(--shuttle-duration);
  }
  :global(a.item:hover) { background: var(--shuttle-bg-subtle); color: var(--shuttle-fg-primary); }
  :global(a.item.on)    { background: var(--shuttle-bg-subtle); color: var(--shuttle-fg-primary); }

  .ico { width: 16px; height: 16px; flex-shrink: 0; display: inline-flex; }

  .footer {
    margin-top: auto;
    padding-top: var(--shuttle-space-2);
    display: flex; gap: var(--shuttle-space-1);
    justify-content: space-between;
    border-top: 1px solid var(--shuttle-border);
  }
</style>
