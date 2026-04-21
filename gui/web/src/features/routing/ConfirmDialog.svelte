<script lang="ts">
  import { Dialog, Button } from '@/ui'
  import { t } from '@/lib/i18n/index'

  interface Props {
    open: boolean
    title: string
    description?: string
    confirmLabel?: string
    cancelLabel?: string
    danger?: boolean
    onConfirm: () => Promise<void> | void
  }
  let {
    open = $bindable(false),
    title,
    description,
    confirmLabel,
    cancelLabel,
    danger = false,
    onConfirm,
  }: Props = $props()

  let busy = $state(false)

  async function confirm() {
    busy = true
    try { await onConfirm(); open = false } finally { busy = false }
  }
</script>

<Dialog bind:open {title} {description}>
  {#snippet actions()}
    <Button variant="ghost" onclick={() => (open = false)}>
      {cancelLabel ?? t('common.cancel')}
    </Button>
    <Button variant={danger ? 'danger' : 'primary'} loading={busy} onclick={confirm}>
      {confirmLabel ?? t('common.save')}
    </Button>
  {/snippet}
</Dialog>
