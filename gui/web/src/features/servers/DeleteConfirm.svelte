<script lang="ts">
  import { Dialog, Button } from '@/ui'
  import { t } from '@/lib/i18n/index'

  interface Props {
    open: boolean
    count: number
    onConfirm: () => Promise<void> | void
  }
  let { open = $bindable(false), count, onConfirm }: Props = $props()

  let busy = $state(false)

  async function confirm() {
    busy = true
    try {
      await onConfirm()
      open = false
    } finally {
      busy = false
    }
  }

  const title = $derived(
    count === 1
      ? t('servers.dialog.delete.titleOne')
      : t('servers.dialog.delete.titleMany', { n: count })
  )
</script>

<Dialog bind:open {title} description={t('common.cannotUndo')}>
  {#snippet actions()}
    <Button variant="ghost" onclick={() => (open = false)}>{t('common.cancel')}</Button>
    <Button variant="danger" loading={busy} onclick={confirm}>{t('common.delete')}</Button>
  {/snippet}
</Dialog>
