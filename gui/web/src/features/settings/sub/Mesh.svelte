<script lang="ts">
  import { Switch } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { settings } from '../config.svelte'
  import Field from '../Field.svelte'

  const mesh = $derived(settings.draft?.mesh)
</script>

<h2>{t('settings.meshVPN')}</h2>

{#if mesh}
  <Field label={t('settings.meshEnabled')} hint={t('settings.meshEnabledHint')}>
    <Switch bind:checked={mesh.enabled} />
  </Field>

  {#if mesh.enabled}
    <Field label={t('settings.meshP2P')} hint={t('settings.meshP2PHint')}>
      <Switch bind:checked={mesh.p2p_enabled} />
    </Field>
  {/if}
{/if}

<style>
  h2 {
    margin: 0 0 var(--shuttle-space-4);
    font-size: var(--shuttle-text-lg);
    font-weight: var(--shuttle-weight-semibold);
    color: var(--shuttle-fg-primary);
  }
</style>
