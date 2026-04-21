<script lang="ts">
  import { Field, Input, Select, Switch } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { settings } from '../config.svelte'
  import PageHeader from '../PageHeader.svelte'

  const dns = $derived(settings.draft?.routing?.dns)

  const viaOptions = [
    { value: 'proxy',  label: t('routing.proxy') },
    { value: 'direct', label: t('routing.direct') },
  ]
</script>

<PageHeader title={t('settings.dns')} />

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

