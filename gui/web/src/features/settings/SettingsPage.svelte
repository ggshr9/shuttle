<script lang="ts">
  import { onMount, type Component } from 'svelte'
  import { useRoute, navigate, Link } from '@/lib/router'
  import { Icon, Spinner } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { settings } from './config.svelte'
  import { subNav, DEFAULT_SLUG } from './nav'
  import UnsavedBar from './UnsavedBar.svelte'

  import General  from './sub/General.svelte'
  import Proxy    from './sub/Proxy.svelte'
  import Mesh     from './sub/Mesh.svelte'
  import Routing  from './sub/Routing.svelte'
  import Dns      from './sub/Dns.svelte'
  import Logging  from './sub/Logging.svelte'
  import Qos      from './sub/Qos.svelte'
  import Backup   from './sub/Backup.svelte'
  import Update   from './sub/Update.svelte'
  import Advanced from './sub/Advanced.svelte'

  const pageMap: Record<string, Component> = {
    general:  General,
    proxy:    Proxy,
    mesh:     Mesh,
    routing:  Routing,
    dns:      Dns,
    logging:  Logging,
    qos:      Qos,
    backup:   Backup,
    update:   Update,
    advanced: Advanced,
  }

  const route = useRoute()
  const slug = $derived.by(() => {
    const parts = route.path.split('/').filter(Boolean)
    return parts[0] === 'settings' ? parts[1] ?? DEFAULT_SLUG : DEFAULT_SLUG
  })
  const CurrentPage = $derived<Component | null>(pageMap[slug] ?? null)

  onMount(() => {
    void settings.ensureLoaded()
    // Redirect bare /settings to the default sub-page
    if (route.path === '/settings') {
      navigate(`/settings/${DEFAULT_SLUG}`, { replace: true })
    }
  })
</script>

<div class="shell">
  <nav class="subnav" aria-label="Settings sections">
    {#each subNav as entry (entry.slug)}
      <Link
        to={`/settings/${entry.slug}`}
        class={'sub-item ' + (slug === entry.slug ? 'on' : '')}
      >
        <span class="ico"><Icon name={entry.icon} size={14} /></span>
        <span>{t(entry.labelKey)}</span>
      </Link>
    {/each}
  </nav>

  <div class="content">
    <UnsavedBar />

    {#if settings.loading}
      <div class="center"><Spinner size={20} /></div>
    {:else if settings.error}
      <p class="error">{settings.error}</p>
    {:else if CurrentPage}
      <CurrentPage />
    {/if}
  </div>
</div>

<style>
  .shell {
    display: grid;
    grid-template-columns: 200px 1fr;
    gap: var(--shuttle-space-6);
    min-height: calc(100vh - 120px);
  }
  .subnav {
    display: flex;
    flex-direction: column;
    gap: 1px;
    padding-right: var(--shuttle-space-2);
    border-right: 1px solid var(--shuttle-border);
  }
  :global(a.sub-item) {
    display: flex;
    align-items: center;
    gap: var(--shuttle-space-3);
    height: 30px;
    padding: 0 var(--shuttle-space-3);
    border-radius: var(--shuttle-radius-sm);
    color: var(--shuttle-fg-secondary);
    font-size: var(--shuttle-text-sm);
    text-decoration: none;
    transition: background var(--shuttle-duration), color var(--shuttle-duration);
  }
  :global(a.sub-item:hover) { background: var(--shuttle-bg-subtle); color: var(--shuttle-fg-primary); }
  :global(a.sub-item.on)    { background: var(--shuttle-bg-subtle); color: var(--shuttle-fg-primary); }
  :global(a.sub-item .ico)  { display: inline-flex; }

  .content {
    max-width: 640px;
    min-width: 0;
  }
  .center { display: flex; justify-content: center; padding: var(--shuttle-space-6); }
  .error { color: var(--shuttle-danger); font-size: var(--shuttle-text-sm); }
</style>
