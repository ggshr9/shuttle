<script lang="ts">
  import { onMount } from 'svelte'
  import { Button, Field, Icon, Input, Select, Switch } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { getLanInfo } from '@/lib/api/endpoints'
  import type { LanInfo } from '@/lib/api/types'
  import { settings } from '../config.svelte'
  import PageHeader from '../PageHeader.svelte'

  const proxy = $derived(settings.draft?.proxy)

  let lanInfo = $state<LanInfo | null>(null)
  let newAppName = $state('')

  const perAppOptions = [
    { value: '',       label: t('settings.perAppDisabled') },
    { value: 'allow',  label: t('settings.perAppAllow') },
    { value: 'deny',   label: t('settings.perAppDeny') },
  ]

  async function refreshLan(): Promise<void> {
    if (!proxy?.allow_lan) { lanInfo = null; return }
    try { lanInfo = await getLanInfo() } catch { lanInfo = null }
  }

  onMount(refreshLan)

  function addApp(): void {
    if (!proxy?.tun) return
    const name = newAppName.trim()
    if (!name) return
    const list = proxy.tun.app_list ?? []
    if (!list.includes(name)) proxy.tun.app_list = [...list, name]
    newAppName = ''
  }

  function removeApp(idx: number): void {
    if (!proxy?.tun?.app_list) return
    proxy.tun.app_list = proxy.tun.app_list.filter((_, i) => i !== idx)
  }
</script>

<PageHeader title={t('settings.proxyListeners')} />

{#if proxy && proxy.socks5 && proxy.http && proxy.tun && proxy.system_proxy}
  <Field label="SOCKS5">
    <Input bind:value={proxy.socks5.listen} placeholder="127.0.0.1:1080" />
    <Switch bind:checked={proxy.socks5.enabled} />
  </Field>

  <Field label="HTTP">
    <Input bind:value={proxy.http.listen} placeholder="127.0.0.1:8080" />
    <Switch bind:checked={proxy.http.enabled} />
  </Field>

  <Field label="TUN">
    <Input bind:value={proxy.tun.device_name} placeholder="utun7" />
    <Switch bind:checked={proxy.tun.enabled} />
  </Field>

  {#if proxy.tun.enabled}
    <Field label={t('settings.perAppRouting')}>
      <Select
        value={proxy.tun.per_app_mode ?? ''}
        options={perAppOptions}
        onValueChange={(v) => { proxy.tun!.per_app_mode = v as '' | 'allow' | 'deny' }}
      />
    </Field>

    {#if proxy.tun.per_app_mode}
      <div class="per-app">
        {#each proxy.tun.app_list ?? [] as app, i (app + i)}
          <div class="app-row">
            <span class="app-name">{app}</span>
            <button type="button" class="app-remove" onclick={() => removeApp(i)} aria-label="Remove">
              <Icon name="x" size={12} />
            </button>
          </div>
        {/each}
        <div class="add-row">
          <Input
            bind:value={newAppName}
            placeholder="com.example.app or process name"
            onchange={(e) => { if ((e as KeyboardEvent).key === 'Enter') addApp() }}
          />
          <Button variant="ghost" onclick={addApp}>{t('settings.add')}</Button>
        </div>
      </div>
    {/if}
  {/if}

  <Field label={t('settings.allowLan')} hint={t('settings.allowLanHint')}>
    <Switch checked={proxy.allow_lan ?? false} onCheckedChange={(v) => { proxy.allow_lan = v; refreshLan() }} />
  </Field>

  {#if proxy.allow_lan && lanInfo?.interfaces?.length}
    <div class="lan-info">
      <div class="lan-title">{t('settings.lanAddresses')}</div>
      {#each lanInfo.interfaces as iface}
        <div class="lan-addr">{iface}</div>
      {/each}
      <div class="lan-hint">{t('settings.lanAddressesHint')}</div>
    </div>
  {/if}

  <Field label={t('settings.autoSystemProxy')} hint={t('settings.autoSystemProxyHint')}>
    <Switch bind:checked={proxy.system_proxy.enabled} />
  </Field>
{/if}

<style>
  .per-app {
    padding: var(--shuttle-space-3) 0;
    border-bottom: 1px solid var(--shuttle-border);
    display: flex;
    flex-direction: column;
    gap: var(--shuttle-space-2);
  }
  .app-row {
    display: flex;
    align-items: center;
    gap: var(--shuttle-space-3);
    padding: var(--shuttle-space-2) var(--shuttle-space-3);
    background: var(--shuttle-bg-surface);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-sm);
  }
  .app-name {
    flex: 1;
    font-family: var(--shuttle-font-mono);
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-primary);
  }
  .app-remove {
    border: none;
    background: transparent;
    color: var(--shuttle-fg-muted);
    cursor: pointer;
    padding: 2px;
    border-radius: 3px;
  }
  .app-remove:hover { color: var(--shuttle-danger); background: var(--shuttle-bg-subtle); }
  .add-row {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: var(--shuttle-space-2);
    margin-top: var(--shuttle-space-1);
  }

  .lan-info {
    margin-top: var(--shuttle-space-3);
    padding: var(--shuttle-space-3);
    background: var(--shuttle-bg-surface);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
  }
  .lan-title {
    font-size: var(--shuttle-text-xs);
    font-weight: var(--shuttle-weight-semibold);
    color: var(--shuttle-fg-muted);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    margin-bottom: var(--shuttle-space-2);
  }
  .lan-addr {
    font-family: var(--shuttle-font-mono);
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-primary);
    padding: 2px 0;
  }
  .lan-hint {
    margin-top: var(--shuttle-space-2);
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-muted);
  }
</style>
