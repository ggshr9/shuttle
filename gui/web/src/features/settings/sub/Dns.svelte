<script lang="ts">
  import { Input, Select, Switch } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { settings } from '../config.svelte'
  import Field from '../Field.svelte'

  const dns = $derived(settings.draft?.routing?.dns)

  const viaOptions = [
    { value: 'proxy',  label: t('routing.proxy') },
    { value: 'direct', label: t('routing.direct') },
  ]
</script>

<h2>{t('settings.dns')}</h2>

{#if dns && dns.remote}
  <Field label={t('settings.domesticDns')}>
    <Input bind:value={dns.domestic} placeholder="223.5.5.5" />
  </Field>

  <Field label={t('settings.remoteDns')}>
    <Input bind:value={dns.remote.server} placeholder="https://1.1.1.1/dns-query" />
  </Field>

  <Field label={t('settings.remoteVia')}>
    <Select
      value={dns.remote.via ?? 'proxy'}
      options={viaOptions}
      onValueChange={(v) => { dns.remote!.via = v as 'proxy' | 'direct' }}
    />
  </Field>

  <Field label={t('settings.enableDnsCache')}>
    <Switch bind:checked={dns.cache} />
  </Field>

  <Field label={t('settings.enableDnsPrefetch')}>
    <Switch bind:checked={dns.prefetch} />
  </Field>
{/if}

<style>
  h2 {
    margin: 0 0 var(--shuttle-space-4);
    font-size: var(--shuttle-text-lg);
    font-weight: var(--shuttle-weight-semibold);
    color: var(--shuttle-fg-primary);
  }
</style>
