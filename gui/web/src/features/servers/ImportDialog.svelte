<script lang="ts">
  import { Dialog, Button } from '@/ui'
  import { importServers } from './resource.svelte'

  interface Props {
    open: boolean
  }
  let { open = $bindable(false) }: Props = $props()

  let data = $state('')
  let submitting = $state(false)

  async function submit() {
    if (!data.trim()) return
    submitting = true
    try {
      const r = await importServers(data)
      if (r && (r.added > 0 || !r.error)) {
        data = ''
        open = false
      }
    } finally {
      submitting = false
    }
  }
</script>

<Dialog bind:open title="Import servers" description="Paste a Clash YAML, Base64 SIP-008 subscription, or shuttle:// URI.">
  <textarea
    class="ta"
    placeholder={'shuttle://...  or  proxies:\n  - name: ...  or  base64 SIP-008'}
    bind:value={data}
  ></textarea>

  {#snippet actions()}
    <Button variant="ghost" onclick={() => (open = false)}>Cancel</Button>
    <Button variant="primary" disabled={!data.trim()} loading={submitting} onclick={submit}>
      Import
    </Button>
  {/snippet}
</Dialog>

<style>
  .ta {
    width: 100%; min-height: 160px;
    padding: var(--shuttle-space-3);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    background: var(--shuttle-bg-surface);
    color: var(--shuttle-fg-primary);
    font-family: var(--shuttle-font-mono);
    font-size: var(--shuttle-text-sm);
    resize: vertical;
    outline: none;
  }
  .ta:focus { border-color: var(--shuttle-border-strong); }
</style>
