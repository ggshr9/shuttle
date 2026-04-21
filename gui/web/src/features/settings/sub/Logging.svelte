<script lang="ts">
  import { Select } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { settings } from '../config.svelte'
  import Field from '../Field.svelte'

  const log = $derived(settings.draft?.log)

  const levelOptions = [
    { value: 'debug', label: t('settings.logDebug') },
    { value: 'info',  label: t('settings.logInfo') },
    { value: 'warn',  label: t('settings.logWarn') },
    { value: 'error', label: t('settings.logError') },
  ]
</script>

<h2>{t('settings.log')}</h2>

{#if log}
  <Field label={t('settings.logLevel')}>
    <Select
      value={log.level ?? 'info'}
      options={levelOptions}
      onValueChange={(v) => { log.level = v as 'debug' | 'info' | 'warn' | 'error' }}
    />
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
