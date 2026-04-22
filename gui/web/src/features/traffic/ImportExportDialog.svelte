<script lang="ts">
  import { Dialog, Button, Icon } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { importRules, exportHref } from './resource.svelte'
  import type { RoutingRules } from '@/lib/api/types'

  interface Props {
    open: boolean
    onImported?: () => void
  }
  let { open = $bindable(false), onImported }: Props = $props()

  let mode = $state<'merge' | 'replace'>('merge')
  let parsed = $state<RoutingRules | null>(null)
  let parseError = $state<string | null>(null)
  let dragOver = $state(false)
  let submitting = $state(false)
  let textareaValue = $state('')

  function parseJson(text: string) {
    parseError = null
    parsed = null
    if (!text.trim()) return
    try {
      const obj = JSON.parse(text)
      if (!obj || !Array.isArray(obj.rules)) {
        parseError = t('routing.importExport.invalidFile')
        return
      }
      parsed = obj as RoutingRules
    } catch (e) {
      parseError = (e as Error).message
    }
  }

  async function onFile(f: File) {
    const text = await f.text()
    textareaValue = text
    parseJson(text)
  }

  function onPaste(ev: Event) {
    const v = (ev.target as HTMLTextAreaElement).value
    textareaValue = v
    parseJson(v)
  }

  async function doImport() {
    if (!parsed) return
    submitting = true
    try {
      const r = await importRules(parsed, mode)
      textareaValue = ''
      parsed = null
      if (r) {
        onImported?.()
        open = false
      }
    } finally {
      submitting = false
    }
  }

  function doExport() {
    window.location.href = exportHref()
  }
</script>

<Dialog bind:open title={t('routing.importExport.title')} description={t('routing.importExport.desc')}>
  <div class="col">
    <div
      class="dropzone"
      class:over={dragOver}
      ondragover={(e) => { e.preventDefault(); dragOver = true }}
      ondragleave={() => (dragOver = false)}
      ondrop={(e) => {
        e.preventDefault()
        dragOver = false
        const f = e.dataTransfer?.files?.[0]
        if (f) onFile(f)
      }}
      role="region"
      aria-label={t('routing.importExport.dropHint')}
    >
      <Icon name="plus" size={20} />
      <span>{t('routing.importExport.dropHint')}</span>
    </div>

    <textarea
      value={textareaValue}
      oninput={onPaste}
      placeholder={t('routing.importExport.pastePlaceholder')}
      rows="5"
    ></textarea>

    {#if parseError}
      <div class="err">{parseError}</div>
    {:else if parsed}
      <div class="ok">{t('routing.importExport.parsed', { n: parsed.rules.length })}</div>
    {/if}

    <div class="mode">
      <label><input type="radio" name="mode" value="merge"  checked={mode === 'merge'}  onchange={() => (mode = 'merge')}  />{t('routing.importExport.merge')}</label>
      <label><input type="radio" name="mode" value="replace" checked={mode === 'replace'} onchange={() => (mode = 'replace')} />{t('routing.importExport.replace')}</label>
    </div>

    <div class="export">
      <Button variant="secondary" onclick={doExport}>{t('routing.importExport.export')}</Button>
      <span class="hint">{t('routing.importExport.exportHint')}</span>
    </div>
  </div>

  {#snippet actions()}
    <Button variant="ghost" onclick={() => (open = false)}>{t('common.cancel')}</Button>
    <Button variant="primary" disabled={!parsed} loading={submitting} onclick={doImport}>
      {t('routing.importExport.import')}
    </Button>
  {/snippet}
</Dialog>

<style>
  .col { display: flex; flex-direction: column; gap: var(--shuttle-space-3); }
  .dropzone {
    display: flex; flex-direction: column; align-items: center; justify-content: center;
    padding: var(--shuttle-space-5);
    border: 2px dashed var(--shuttle-border-strong);
    border-radius: var(--shuttle-radius-md);
    color: var(--shuttle-fg-muted);
    font-size: var(--shuttle-text-sm);
    gap: var(--shuttle-space-2);
    transition: border-color var(--shuttle-duration);
  }
  .dropzone.over { border-color: var(--shuttle-accent); color: var(--shuttle-fg-primary); }
  textarea {
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    background: var(--shuttle-bg-surface);
    color: var(--shuttle-fg-primary);
    padding: var(--shuttle-space-3);
    font-family: var(--shuttle-font-mono);
    font-size: var(--shuttle-text-xs);
    outline: none;
    resize: vertical;
  }
  textarea:focus { border-color: var(--shuttle-border-strong); }
  .err { color: var(--shuttle-danger); font-size: var(--shuttle-text-sm); }
  .ok  { color: var(--shuttle-success); font-size: var(--shuttle-text-sm); }
  .mode { display: flex; gap: var(--shuttle-space-3); font-size: var(--shuttle-text-sm); }
  .mode label { display: flex; align-items: center; gap: var(--shuttle-space-1); cursor: pointer; }
  .export {
    display: flex; align-items: center; gap: var(--shuttle-space-2);
    border-top: 1px solid var(--shuttle-border);
    padding-top: var(--shuttle-space-3);
  }
  .hint { font-size: var(--shuttle-text-xs); color: var(--shuttle-fg-muted); }
</style>
