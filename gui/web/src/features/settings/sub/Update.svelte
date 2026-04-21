<script lang="ts">
  import { onMount } from 'svelte'
  import { Badge, Button, Field } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { getVersion, checkUpdate } from '@/lib/api/endpoints'
  import type { UpdateInfo } from '@/lib/api/types'
  import { formatBytes } from '@/lib/format'
  import PageHeader from '../PageHeader.svelte'

  let version = $state('')
  let info = $state<UpdateInfo | null>(null)
  let checking = $state(false)
  let showLog = $state(false)

  onMount(async () => {
    try { version = (await getVersion()).version } catch { version = 'unknown' }
    await check(false)
  })

  async function check(force: boolean): Promise<void> {
    checking = true
    try { info = await checkUpdate(force) } catch { /* tolerate */ }
    finally { checking = false }
  }
</script>

<PageHeader title={t('settings.nav.update')} />

<Field label={t('settings.currentVersion')}>
  <code class="version">{version || '—'}</code>
</Field>

{#if info?.available}
  <div class="banner">
    <div class="banner-head">
      <Badge variant="success">{t('settings.newVersion')}</Badge>
      <span class="latest">{info.latest_version}</span>
    </div>
    <div class="banner-actions">
      {#if info.changelog}
        <Button variant="ghost" onclick={() => (showLog = !showLog)}>
          {showLog ? t('settings.hideChangelog') : 'Changelog'}
        </Button>
      {/if}
      {#if info.release_url}
        <a class="btn-link" href={info.release_url} target="_blank" rel="noopener noreferrer">
          Release
        </a>
      {/if}
      {#if info.download_url}
        <a class="btn-link primary" href={info.download_url}>
          {t('settings.download')}{info.asset_size ? ` (${formatBytes(info.asset_size)})` : ''}
        </a>
      {/if}
    </div>
    {#if showLog && info.changelog}
      <pre class="log">{info.changelog}</pre>
    {/if}
  </div>
{:else if info}
  <p class="ok">✓ Up to date</p>
{/if}

<Field label={t('settings.checkUpdates')}>
  <Button variant="ghost" loading={checking} onclick={() => check(true)}>
    {checking ? t('settings.checking') : t('settings.checkUpdates')}
  </Button>
</Field>

<style>
  .version {
    font-family: var(--shuttle-font-mono);
    font-size: var(--shuttle-text-sm);
    color: var(--shuttle-fg-primary);
  }
  .banner {
    margin: var(--shuttle-space-3) 0;
    padding: var(--shuttle-space-4);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    background: var(--shuttle-bg-surface);
    display: flex;
    flex-direction: column;
    gap: var(--shuttle-space-3);
  }
  .banner-head { display: flex; align-items: center; gap: var(--shuttle-space-3); }
  .latest {
    font-family: var(--shuttle-font-mono);
    font-size: var(--shuttle-text-sm);
    color: var(--shuttle-fg-primary);
  }
  .banner-actions { display: flex; gap: var(--shuttle-space-2); flex-wrap: wrap; }
  .btn-link {
    display: inline-flex;
    align-items: center;
    padding: 0 var(--shuttle-space-3);
    height: 32px;
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    color: var(--shuttle-fg-primary);
    background: var(--shuttle-bg-surface);
    text-decoration: none;
    font-size: var(--shuttle-text-sm);
  }
  .btn-link:hover { border-color: var(--shuttle-border-strong); }
  .btn-link.primary {
    background: var(--shuttle-accent);
    color: var(--shuttle-accent-fg);
    border-color: var(--shuttle-accent);
  }
  .log {
    margin: 0;
    padding: var(--shuttle-space-3);
    background: var(--shuttle-bg-base);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-sm);
    font-family: var(--shuttle-font-mono);
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-primary);
    max-height: 240px;
    overflow-y: auto;
    white-space: pre-wrap;
  }
  .ok {
    margin: var(--shuttle-space-3) 0;
    font-size: var(--shuttle-text-sm);
    color: var(--shuttle-success);
  }
</style>
