<script lang="ts">
  import { Field, Select } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { settings } from '../config.svelte'
  import PageHeader from '../PageHeader.svelte'

  const log = $derived(settings.draft?.log)

  const levelOptions = [
    { value: 'debug', label: t('settings.logDebug') },
    { value: 'info',  label: t('settings.logInfo') },
    { value: 'warn',  label: t('settings.logWarn') },
    { value: 'error', label: t('settings.logError') },
  ]
</script>

<PageHeader title={t('settings.log')} />

{#if log}
  <Field label={t('settings.logLevel')}>
    <Select
      value={log.level ?? 'info'}
      options={levelOptions}
      onValueChange={(v) => { log.level = v as 'debug' | 'info' | 'warn' | 'error' }}
    />
  </Field>
{/if}

