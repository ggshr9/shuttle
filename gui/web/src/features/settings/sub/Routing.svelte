<script lang="ts">
  import { onMount } from 'svelte'
  import { Button, Field, Select, Switch } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { getGeoDataStatus, updateGeoData } from '@/lib/api/endpoints'
  import type { GeoDataStatus } from '@/lib/api/types'
  import { toasts } from '@/lib/toaster.svelte'
  import { settings } from '../config.svelte'
  import PageHeader from '../PageHeader.svelte'

  const routing = $derived(settings.draft?.routing)
  let geo = $state<GeoDataStatus | null>(null)
  let updating = $state(false)

  async function refresh(): Promise<void> {
    try { geo = await getGeoDataStatus() } catch { /* tolerated */ }
  }

  async function runUpdate(): Promise<void> {
    updating = true
    try { geo = await updateGeoData() }
    catch (e) { toasts.error((e as Error).message) }
    finally { updating = false }
  }

  onMount(refresh)

  const defaultOptions = [
    { value: 'proxy',  label: t('routing.proxy') },
    { value: 'direct', label: t('routing.direct') },
    { value: 'reject', label: t('routing.reject') },
  ]

  function formatLast(ts?: string): string {
    if (!ts || ts === '0001-01-01T00:00:00Z') return t('settings.geodataNever')
    return new Date(ts).toLocaleString()
  }
</script>

<PageHeader title={t('nav.routing')} />

{#if routing && routing.geodata}
  <Field label={t('routing.defaultAction')}>
    <Select
      value={routing.default ?? 'proxy'}
      options={defaultOptions}
      onValueChange={(v) => { routing.default = v as 'proxy' | 'direct' | 'reject' }}
    />
  </Field>

  <h3>{t('settings.geodata')}</h3>

  <Field label={t('settings.geodataEnabled')} hint={t('settings.geodataHint')}>
    <Switch bind:checked={routing.geodata.enabled} />
  </Field>

  {#if routing.geodata.enabled}
    <Field label={t('settings.geodataAutoUpdate')}>
      <Switch bind:checked={routing.geodata.auto_update} />
    </Field>

    <div class="status">
      <div class="row">
        <span class="k">{t('settings.geodataFiles')}</span>
        <span class="v">{geo?.files_present?.length ?? 0} / 6</span>
      </div>
      <div class="row">
        <span class="k">{t('settings.geodataLastUpdate')}</span>
        <span class="v" class:warn={!geo?.last_update || geo.last_update === '0001-01-01T00:00:00Z'}>
          {formatLast(geo?.last_update)}
        </span>
      </div>
      {#if geo?.last_error}
        <div class="row">
          <span class="k err">{t('settings.geodataError')}</span>
          <span class="v err">{geo.last_error}</span>
        </div>
      {/if}
      <Button variant="ghost" loading={updating} onclick={runUpdate}>
        {updating ? t('settings.geodataUpdating') : t('settings.geodataUpdateNow')}
      </Button>
    </div>
  {/if}
{/if}

<style>
  h3 {
    margin: var(--shuttle-space-5) 0 var(--shuttle-space-3);
    font-size: var(--shuttle-text-sm);
    font-weight: var(--shuttle-weight-semibold);
    color: var(--shuttle-fg-muted);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .status {
    display: flex;
    flex-direction: column;
    gap: var(--shuttle-space-2);
    padding: var(--shuttle-space-4);
    background: var(--shuttle-bg-surface);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    margin-top: var(--shuttle-space-3);
  }
  .row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    font-size: var(--shuttle-text-sm);
  }
  .k { color: var(--shuttle-fg-muted); }
  .v { color: var(--shuttle-fg-primary); }
  .v.warn { color: var(--shuttle-warning); }
  .k.err, .v.err { color: var(--shuttle-danger); }
</style>
