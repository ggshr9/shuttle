<script lang="ts">
  import { Dialog, Input, Button } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { addSubscription } from './resource.svelte'

  interface Props { open: boolean }
  let { open = $bindable(false) }: Props = $props()

  let name = $state('')
  let url = $state('')
  let submitting = $state(false)

  const canSubmit = $derived(url.trim().length > 0)

  async function submit() {
    if (!canSubmit) return
    submitting = true
    try {
      await addSubscription(name.trim(), url.trim())
      name = ''; url = ''
      open = false
    } finally {
      submitting = false
    }
  }
</script>

<Dialog bind:open title={t('subscriptions.dialog.add.title')} description={t('subscriptions.dialog.add.desc')}>
  <div class="fields">
    <Input label={t('subscriptions.name')} bind:value={name} />
    <Input label={t('subscriptions.url')} placeholder="https://example.com/sub.yaml" bind:value={url} />
  </div>

  {#snippet actions()}
    <Button variant="ghost" onclick={() => (open = false)}>{t('common.cancel')}</Button>
    <Button variant="primary" disabled={!canSubmit} loading={submitting} onclick={submit}>
      {t('subscriptions.add')}
    </Button>
  {/snippet}
</Dialog>

<style>
  .fields { display: flex; flex-direction: column; gap: var(--shuttle-space-3); }
</style>
