<script lang="ts">
  import { t } from '../i18n/index'
  import { api, type RoutingRules } from '../api'

  let {
    onImportComplete,
    onMessage,
  } = $props()

  let importing = $state(false)
  let importMode = $state<'merge' | 'replace'>('merge')
  let dragOver = $state(false)
  let droppedFileName = $state('')
  let droppedRules = $state<RoutingRules | null>(null)
  let importData = $state('')
  let importError = $state('')
  let showImportModal = $state(false)

  function validateRulesData(data: unknown): data is RoutingRules {
    if (!data || typeof data !== 'object') return false
    if (!Array.isArray((data as RoutingRules).rules)) return false
    return true
  }

  function handleDragOver(e: DragEvent) {
    e.preventDefault()
    dragOver = true
  }

  function handleDragLeave(e: DragEvent) {
    e.preventDefault()
    dragOver = false
  }

  function handleDrop(e: DragEvent) {
    e.preventDefault()
    dragOver = false
    importError = ''
    const file = e.dataTransfer?.files?.[0]
    if (!file) return
    if (!file.name.endsWith('.json')) {
      importError = 'Only .json files are supported'
      return
    }
    const reader = new FileReader()
    reader.onload = () => {
      try {
        const parsed = JSON.parse(reader.result as string)
        if (!validateRulesData(parsed)) {
          importError = 'Invalid file: expected JSON with a "rules" array'
          droppedRules = null
          droppedFileName = ''
          return
        }
        droppedRules = parsed
        droppedFileName = file.name
        importData = JSON.stringify(parsed, null, 2)
        importError = ''
      } catch {
        importError = 'Failed to parse JSON file'
        droppedRules = null
        droppedFileName = ''
      }
    }
    reader.readAsText(file)
  }

  async function doImport() {
    importing = true
    importError = ''
    try {
      let parsed: unknown
      if (droppedRules) {
        parsed = droppedRules
      } else if (importData.trim()) {
        parsed = JSON.parse(importData) as unknown
      } else {
        return
      }
      if (!validateRulesData(parsed)) {
        importError = 'Invalid file: expected JSON with a "rules" array'
        return
      }
      const result = await api.importRouting(parsed, importMode)
      showImportModal = false
      importData = ''
      droppedRules = null
      droppedFileName = ''
      importMode = 'merge'
      onMessage(`Imported ${result.added} rule(s)`)
      onImportComplete()
    } catch (e) {
      importError = 'Import failed: ' + (e instanceof Error ? e.message : String(e))
    } finally {
      importing = false
    }
  }

  async function exportRules() {
    try {
      const data = await api.exportRoutingData()
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
      const url = window.URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = 'shuttle-rules.json'
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      window.URL.revokeObjectURL(url)
      onMessage('Rules exported')
    } catch (e) {
      onMessage('Export failed: ' + (e instanceof Error ? e.message : String(e)))
    }
  }

  function closeImportModal() {
    showImportModal = false
    importData = ''
    droppedRules = null
    droppedFileName = ''
    importError = ''
    importMode = 'merge'
  }
</script>

<div class="import-export-section">
  <h3 class="section-title">{t('routing.importExport')}</h3>
  <div
    class="drop-zone"
    class:drop-zone-active={dragOver}
    class:drop-zone-has-file={!!droppedFileName}
    ondragover={handleDragOver}
    ondragleave={handleDragLeave}
    ondrop={handleDrop}
    role="region"
    aria-label="Drop zone for importing rule files"
  >
    {#if droppedFileName}
      <span class="drop-file-icon">&#128196;</span>
      <span class="drop-file-name">{droppedFileName}</span>
      <span class="drop-file-hint">{droppedRules?.rules?.length ?? 0} rule(s) found</span>
      <button class="drop-clear" onclick={() => { droppedFileName = ''; droppedRules = null; importData = ''; importError = '' }}>{t('routing.clear')}</button>
    {:else}
      <span class="drop-icon">&#8615;</span>
      <span class="drop-text">{t('routing.dragDrop')}</span>
      <span class="drop-hint">{t('routing.dragDropHint')}</span>
    {/if}
  </div>
  {#if importError}
    <p class="import-error">{importError}</p>
  {/if}
  <div class="import-mode-row">
    <label class="mode-label">
      <input type="radio" name="importMode" value="merge" bind:group={importMode} />
      {t('routing.mergeMode')}
    </label>
    <label class="mode-label">
      <input type="radio" name="importMode" value="replace" bind:group={importMode} />
      {t('routing.replaceMode')}
    </label>
  </div>
  <div class="import-export-actions">
    <button class="btn-import-action" onclick={() => { if (droppedRules) { doImport() } else { showImportModal = true } }} disabled={importing}>
      {importing ? t('routing.importing') : t('routing.import')}
    </button>
    <button class="btn-export-action" onclick={exportRules}>
      {t('routing.export')}
    </button>
  </div>
</div>

{#if showImportModal}
<div class="overlay" onclick={closeImportModal} role="dialog" aria-modal="true" aria-labelledby="import-rules-dialog-title" onkeydown={(e) => e.key === 'Escape' && closeImportModal()}>
  <div class="modal" onclick={(e) => e.stopPropagation()}>
    <h3 id="import-rules-dialog-title">{t('routing.importRules')}</h3>
    <p class="modal-hint">{t('routing.importRulesHint')}</p>
    <textarea
      bind:value={importData}
      placeholder={'{"rules": [{"geosite": "cn", "action": "direct"}], "default": "proxy"}'}
      rows="8"
    ></textarea>
    {#if importError}
      <p class="import-error">{importError}</p>
    {/if}
    <div class="import-mode-row modal-mode-row">
      <label class="mode-label">
        <input type="radio" name="modalImportMode" value="merge" bind:group={importMode} />
        {t('routing.merge')}
      </label>
      <label class="mode-label">
        <input type="radio" name="modalImportMode" value="replace" bind:group={importMode} />
        {t('routing.replace')}
      </label>
    </div>
    <div class="modal-actions">
      <button class="close-btn" onclick={closeImportModal}>{t('common.cancel')}</button>
      <button class="apply-btn" onclick={doImport} disabled={importing || !importData.trim()}>
        {importing ? t('routing.importing') : t('routing.import')}
      </button>
    </div>
  </div>
</div>
{/if}

<style>
  .import-export-section {
    margin-top: 24px;
    padding: 16px;
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 8px;
  }

  .section-title {
    font-size: 14px;
    font-weight: 600;
    margin: 0 0 12px;
    color: var(--text-primary);
  }

  .drop-zone {
    border: 2px dashed var(--border);
    border-radius: 8px;
    padding: 24px;
    text-align: center;
    cursor: default;
    transition: border-color 0.2s, background 0.2s;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 4px;
  }

  .drop-zone-active {
    border-color: var(--accent);
    background: rgba(88, 166, 255, 0.06);
  }

  .drop-zone-has-file {
    border-color: var(--accent-green);
    border-style: solid;
    background: rgba(63, 185, 80, 0.06);
  }

  .drop-icon {
    font-size: 24px;
    color: var(--text-muted);
    line-height: 1;
  }

  .drop-text {
    font-size: 13px;
    color: var(--text-secondary);
    font-weight: 500;
  }

  .drop-hint {
    font-size: 11px;
    color: var(--text-muted);
  }

  .drop-file-icon {
    font-size: 20px;
    line-height: 1;
  }

  .drop-file-name {
    font-size: 13px;
    color: var(--accent-green);
    font-weight: 500;
  }

  .drop-file-hint {
    font-size: 11px;
    color: var(--text-secondary);
  }

  .drop-clear {
    background: none;
    border: none;
    color: var(--accent-red);
    font-size: 11px;
    cursor: pointer;
    padding: 2px 6px;
    margin-top: 4px;
  }

  .drop-clear:hover {
    text-decoration: underline;
  }

  .import-error {
    font-size: 12px;
    color: var(--accent-red);
    margin: 8px 0 0;
  }

  .import-mode-row {
    display: flex;
    gap: 16px;
    margin-top: 12px;
  }

  .modal-mode-row {
    margin-top: 8px;
    margin-bottom: 0;
  }

  .mode-label {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--text-secondary);
    cursor: pointer;
  }

  .mode-label input[type="radio"] {
    accent-color: var(--accent);
  }

  .import-export-actions {
    display: flex;
    gap: 8px;
    margin-top: 12px;
  }

  .btn-import-action {
    background: var(--bg-tertiary);
    color: var(--accent);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px 16px;
    cursor: pointer;
    font-size: 13px;
  }

  .btn-import-action:hover { background: #30363d; }
  .btn-import-action:disabled { opacity: 0.5; cursor: default; }

  .btn-export-action {
    background: var(--bg-tertiary);
    color: var(--accent-green);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px 16px;
    cursor: pointer;
    font-size: 13px;
  }

  .btn-export-action:hover { background: #30363d; }

  /* Modal styles */
  .overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
  }

  .modal {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 20px;
    width: 450px;
    max-height: 500px;
    display: flex;
    flex-direction: column;
  }

  .modal h3 { font-size: 16px; margin: 0 0 4px; color: var(--text-primary); }
  .modal-hint { font-size: 12px; color: var(--text-muted); margin: 0 0 12px; }

  .modal textarea {
    width: 100%;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 10px;
    color: var(--text-primary);
    font-size: 12px;
    font-family: 'Cascadia Code', 'Fira Code', monospace;
    resize: vertical;
    box-sizing: border-box;
  }

  .modal textarea:focus { outline: none; border-color: var(--accent); }

  .modal-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 12px;
  }

  .close-btn {
    margin-top: 12px;
    background: var(--bg-tertiary);
    color: var(--text-primary);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px;
    cursor: pointer;
    font-size: 13px;
  }

  .apply-btn {
    background: var(--btn-bg);
    color: #fff;
    border: none;
    border-radius: 6px;
    padding: 8px 16px;
    cursor: pointer;
    font-size: 13px;
  }

  .apply-btn:hover { background: var(--btn-bg-hover); }
  .apply-btn:disabled { opacity: 0.5; }
</style>
