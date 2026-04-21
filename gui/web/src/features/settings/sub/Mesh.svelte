<script lang="ts">
  import { Field, Switch } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { settings } from '../config.svelte'
  import PageHeader from '../PageHeader.svelte'

  const mesh = $derived(settings.draft?.mesh)
</script>

<PageHeader title={t('settings.meshVPN')} />

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

