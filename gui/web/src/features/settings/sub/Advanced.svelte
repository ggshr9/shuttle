<script lang="ts">
  import { Button, Field } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { exportConfig, downloadDiagnostics } from '@/lib/api/endpoints'
  import { toasts } from '@/lib/toaster.svelte'
  import PageHeader from '../PageHeader.svelte'

  let downloadingDiag = $state(false)

  async function runDiag(): Promise<void> {
    downloadingDiag = true
    try { await downloadDiagnostics() }
    catch (e) { toasts.error((e as Error).message) }
    finally { downloadingDiag = false }
  }
</script>

<PageHeader title={t('settings.nav.advanced')} />

<h3>{t('settings.export')}</h3>

<Field label={t('settings.exportJson')}>
  <a class="btn-link" href={exportConfig('json')} download>
    {t('settings.exportJson')}
  </a>
</Field>

<Field label={t('settings.exportUri')}>
  <a class="btn-link" href={exportConfig('uri')} download>
    {t('settings.exportUri')}
  </a>
</Field>

<h3>{t('settings.diagnostics')}</h3>

<p class="hint">{t('settings.diagnosticsDesc')}</p>

<Field label={t('settings.downloadDiagnostics')}>
  <Button variant="ghost" loading={downloadingDiag} onclick={runDiag}>
    {downloadingDiag ? t('settings.downloading') : t('settings.downloadDiagnostics')}
  </Button>
</Field>

<style>
  h3 {
    margin: var(--shuttle-space-5) 0 var(--shuttle-space-2);
    font-size: var(--shuttle-text-xs);
    font-weight: var(--shuttle-weight-semibold);
    color: var(--shuttle-fg-muted);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .hint {
    margin: 0 0 var(--shuttle-space-2);
    font-size: var(--shuttle-text-sm);
    color: var(--shuttle-fg-muted);
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
  }
  .btn-link:hover { border-color: var(--shuttle-border-strong); }
</style>
