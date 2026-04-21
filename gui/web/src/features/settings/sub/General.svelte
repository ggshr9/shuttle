<script lang="ts">
  import { onMount } from 'svelte'
  import { Select, Switch } from '@/ui'
  import { t, getLocales, getLocale, setLocale } from '@/lib/i18n/index'
  import { theme } from '@/lib/theme.svelte'
  import { getAutostart, setAutostart } from '@/lib/api/endpoints'
  import { toasts } from '@/lib/toaster.svelte'
  import Field from '../Field.svelte'

  let currentLocale = $state(getLocale())
  let autostart = $state(false)
  let autostartSupported = $state(true)

  const localeOptions = getLocales().map((l) => ({ value: l.code, label: l.name }))
  const themeOptions = [
    { value: 'dark',  label: t('settings.dark') },
    { value: 'light', label: t('settings.light') },
  ]

  onMount(async () => {
    try {
      const s = await getAutostart()
      autostart = s.enabled
    } catch {
      autostartSupported = false
    }
  })

  function onLocaleChange(v: string): void {
    if (v === 'en' || v === 'zh-CN') {
      setLocale(v)
      currentLocale = v
    }
  }

  async function onAutostartChange(next: boolean): Promise<void> {
    try {
      await setAutostart(next)
      autostart = next
    } catch (e) {
      toasts.error((e as Error).message)
    }
  }
</script>

<h2>{t('settings.general')}</h2>

<Field label={t('settings.language')}>
  <Select value={currentLocale} options={localeOptions} onValueChange={onLocaleChange} />
</Field>

<Field label="Theme">
  <Select
    value={theme.current}
    options={themeOptions}
    onValueChange={(v) => theme.set(v as 'dark' | 'light')}
  />
</Field>

{#if autostartSupported}
  <Field label={t('settings.launchAtLogin')} hint={t('settings.launchAtLoginHint')}>
    <Switch checked={autostart} onCheckedChange={onAutostartChange} />
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
