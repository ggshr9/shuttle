<script lang="ts">
  import { Dialog, Button } from '@/ui'

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

  const title = $derived(count === 1 ? 'Delete server?' : `Delete ${count} servers?`)
  const description = 'This cannot be undone.'
</script>

<Dialog bind:open {title} {description}>
  {#snippet actions()}
    <Button variant="ghost" onclick={() => (open = false)}>Cancel</Button>
    <Button variant="danger" loading={busy} onclick={confirm}>Delete</Button>
  {/snippet}
</Dialog>
