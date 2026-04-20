<script lang="ts">
  import { Dialog, Input, Button } from '@/ui'
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

<Dialog bind:open title="Add server" description="Enter server details. Address and password are required.">
  <div class="fields">
    <Input label="Name" placeholder="sg-hk-02" bind:value={name} />
    <Input label="Address" placeholder="example.com:443" bind:value={addr} />
    <Input label="Password" type="password" bind:value={password} />
    <Input label="SNI (optional)" placeholder="example.com" bind:value={sni} />
  </div>

  {#snippet actions()}
    <Button variant="ghost" onclick={() => (open = false)}>Cancel</Button>
    <Button variant="primary" disabled={!canSubmit} loading={submitting} onclick={submit}>
      Add
    </Button>
  {/snippet}
</Dialog>

<style>
  .fields { display: flex; flex-direction: column; gap: var(--shuttle-space-3); }
</style>
