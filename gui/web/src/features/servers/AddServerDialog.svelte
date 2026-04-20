<script lang="ts">
  import { Dialog, Input, Button } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { addServer } from './resource.svelte'

  interface Props {
    open: boolean
  }
  let { open = $bindable(false) }: Props = $props()

  let name = $state('')
  let addr = $state('')
  let password = $state('')
  let sni = $state('')
  let submitting = $state(false)

  const canSubmit = $derived(addr.trim().length > 0 && password.length > 0)

  async function submit() {
    if (!canSubmit) return
    submitting = true
    try {
      await addServer({
        name: name.trim() || undefined,
        addr: addr.trim(),
        password,
        sni: sni.trim() || undefined,
      })
      name = ''; addr = ''; password = ''; sni = ''
      open = false
    } finally {
      submitting = false
    }
  }
</script>

<Dialog bind:open title={t('servers.addServer')} description={t('servers.dialog.add.desc')}>
  <div class="fields">
    <Input label={t('servers.name')} placeholder={t('servers.dialog.add.namePlaceholder')} bind:value={name} />
    <Input label={t('servers.columns.address')} placeholder={t('servers.dialog.add.addrPlaceholder')} bind:value={addr} />
    <Input label={t('servers.password')} type="password" bind:value={password} />
    <Input label={t('servers.dialog.add.sniLabel')} placeholder={t('servers.dialog.add.sniPlaceholder')} bind:value={sni} />
  </div>

  {#snippet actions()}
    <Button variant="ghost" onclick={() => (open = false)}>{t('common.cancel')}</Button>
    <Button variant="primary" disabled={!canSubmit} loading={submitting} onclick={submit}>
      {t('servers.add')}
    </Button>
  {/snippet}
</Dialog>

<style>
  .fields { display: flex; flex-direction: column; gap: var(--shuttle-space-3); }
</style>
