<script lang="ts">
  import { Dialog, Button, Input } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { platform } from '@/lib/platform'
  import { addServer, addSubscription, importConfig } from '@/lib/api/endpoints'
  import { invalidate } from '@/lib/resource.svelte'
  import { toasts } from '@/lib/toaster.svelte'
  import { errorMessage } from '@/lib/format'

  interface Props {
    open: boolean
  }
  let { open = $bindable(false) }: Props = $props()

  type Method = 'manual' | 'paste' | 'subscribe'
  let method = $state<Method>('manual')

  // Manual fields
  let addr = $state('')
  let password = $state('')
  let name = $state('')

  // Paste / Subscribe fields
  let pasteData = $state('')
  let subUrl = $state('')

  let busy = $state(false)

  const canScan = $derived(platform.name === 'native')

  const dialogTitle = $derived(
    method === 'subscribe' ? t('subscriptions.add') :
    method === 'paste'     ? t('servers.dialog.import.title') :
                             t('servers.addServer')
  )

  function reset() {
    method = 'manual'
    addr = ''; password = ''; name = ''
    pasteData = ''; subUrl = ''
    busy = false
  }

  function close() { open = false }

  // Reset whenever the dialog closes, regardless of how (Cancel button,
  // ESC, backdrop click). Prevents ghost text on reopen.
  $effect(() => {
    if (!open) reset()
  })

  async function scan() {
    const r = await platform.scanQRCode()
    if (r === 'unsupported') {
      toasts.error('QR scan unavailable on this device')
      return
    }
    pasteData = r
    method = 'paste'
  }

  async function submit() {
    busy = true
    try {
      if (method === 'manual') {
        if (!addr.trim()) { toasts.error('Address is required'); return }
        await addServer({
          addr: addr.trim(),
          password: password.trim() || undefined,
          name: name.trim() || undefined,
        })
      } else if (method === 'paste') {
        if (!pasteData.trim()) { toasts.error('Paste something first'); return }
        await importConfig(pasteData.trim())
      } else if (method === 'subscribe') {
        if (!subUrl.trim()) { toasts.error('Subscription URL is required'); return }
        await addSubscription('', subUrl.trim())
      }
      invalidate('servers.list')
      invalidate('servers.subscriptions')
      toasts.success(t('servers.toast.added', { name: name || subUrl || 'server' }))
      close()
    } catch (e) {
      toasts.error(errorMessage(e))
    } finally {
      busy = false
    }
  }
</script>

<Dialog bind:open title={dialogTitle}>
  <div class="sheet">
    <div class="tabs" role="tablist">
      <button
        class:active={method === 'manual'}
        role="tab"
        aria-selected={method === 'manual'}
        onclick={() => (method = 'manual')}
      >Manual</button>
      <button
        class:active={method === 'paste'}
        role="tab"
        aria-selected={method === 'paste'}
        onclick={() => (method = 'paste')}
      >Paste</button>
      <button
        class:active={method === 'subscribe'}
        role="tab"
        aria-selected={method === 'subscribe'}
        onclick={() => (method = 'subscribe')}
      >Subscribe</button>
      {#if canScan}
        <button onclick={scan} aria-label="Scan QR code">Scan QR</button>
      {/if}
    </div>

    {#if method === 'manual'}
      <Input label={t('servers.columns.address')} placeholder="example.com:443" bind:value={addr} />
      <Input label={t('servers.password')} type="password" bind:value={password} />
      <Input label={t('servers.name')} placeholder="optional" bind:value={name} />
    {:else if method === 'paste'}
      <label class="paste-label">
        <span>Paste shuttle:// URI, YAML, or JSON</span>
        <textarea bind:value={pasteData} rows="6" placeholder="shuttle://…  or  proxies:\n  - name: …"></textarea>
      </label>
    {:else if method === 'subscribe'}
      <Input label={t('subscriptions.url')} placeholder="https://…" bind:value={subUrl} />
    {/if}
  </div>

  {#snippet actions()}
    <Button variant="ghost" onclick={close}>{t('common.cancel')}</Button>
    <Button variant="primary" loading={busy} onclick={submit}>{t('servers.add')}</Button>
  {/snippet}
</Dialog>

<style>
  .sheet { display: flex; flex-direction: column; gap: var(--shuttle-space-3); min-width: 320px; }
  .tabs { display: flex; gap: var(--shuttle-space-2); flex-wrap: wrap; }
  .tabs button {
    background: transparent;
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-sm);
    padding: var(--shuttle-space-1) var(--shuttle-space-3);
    color: var(--shuttle-fg-secondary);
    cursor: pointer;
    min-height: 36px;
    font-size: var(--shuttle-text-sm);
  }
  .tabs button.active {
    background: var(--shuttle-accent);
    color: var(--shuttle-accent-fg, #fff);
    border-color: var(--shuttle-accent);
  }
  .paste-label { display: flex; flex-direction: column; gap: var(--shuttle-space-1); }
  .paste-label > span {
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-muted);
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }
  textarea {
    width: 100%;
    background: var(--shuttle-bg-subtle);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-sm);
    padding: var(--shuttle-space-2);
    color: var(--shuttle-fg-primary);
    font-family: var(--shuttle-font-mono);
    font-size: var(--shuttle-text-sm);
    resize: vertical;
  }
</style>
