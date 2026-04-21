<script lang="ts">
  import { Button } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { backupUrl, restore } from '@/lib/api/endpoints'
  import { toasts } from '@/lib/toaster.svelte'

  let restoring = $state(false)
  let fileInput: HTMLInputElement | null = $state(null)

  async function handleFile(e: Event): Promise<void> {
    const target = e.target as HTMLInputElement
    const file = target.files?.[0]
    if (!file) return
    restoring = true
    try {
      const backup = JSON.parse(await file.text()) as unknown
      await restore(backup)
      toasts.success(t('settings.restored', { servers: 0, subscriptions: 0 }))
    } catch (err) {
      toasts.error(`${t('settings.restoreFailed')}: ${(err as Error).message}`)
    } finally {
      restoring = false
      target.value = ''
    }
  }
</script>

<h2>{t('settings.backup')}</h2>

<p class="hint">{t('settings.backupHint')}</p>

<div class="actions">
  <a class="btn-link" href={backupUrl()} download="shuttle-backup.json">
    {t('settings.createBackup')}
  </a>
  <Button
    variant="ghost"
    loading={restoring}
    onclick={() => fileInput?.click()}
  >
    {restoring ? t('settings.restoring') : t('settings.restoreBackup')}
  </Button>
  <input
    bind:this={fileInput}
    type="file"
    accept=".json"
    onchange={handleFile}
    hidden
  />
</div>

<style>
  h2 {
    margin: 0 0 var(--shuttle-space-2);
    font-size: var(--shuttle-text-lg);
    font-weight: var(--shuttle-weight-semibold);
    color: var(--shuttle-fg-primary);
  }
  .hint {
    margin: 0 0 var(--shuttle-space-4);
    font-size: var(--shuttle-text-sm);
    color: var(--shuttle-fg-muted);
  }
  .actions {
    display: flex;
    gap: var(--shuttle-space-2);
    flex-wrap: wrap;
  }
  .btn-link {
    display: inline-flex;
    align-items: center;
    padding: 0 var(--shuttle-space-3);
    height: 32px;
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    background: var(--shuttle-bg-surface);
    color: var(--shuttle-fg-primary);
    text-decoration: none;
    font-size: var(--shuttle-text-sm);
    font-weight: var(--shuttle-weight-medium);
  }
  .btn-link:hover { border-color: var(--shuttle-border-strong); }
</style>
