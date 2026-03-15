<script lang="ts">
  import { api } from '../lib/api'
  import { t } from '../lib/i18n/index'
  import { onMount } from 'svelte'

  let routing = $state({ rules: [], default: 'proxy', dns: {} })
  let saving = $state(false)
  let msg = $state('')
  let processes = $state([])
  let showProcessPicker = $state(false)
  let pickerTargetIndex = $state(-1)

  // Templates
  let templates = $state([])
  let showTemplates = $state(false)
  let applyingTemplate = $state(false)

  // Import
  let showImport = $state(false)
  let importData = $state('')
  let importing = $state(false)
  let importMode = $state<'merge' | 'replace'>('merge')
  let dragOver = $state(false)
  let droppedFileName = $state('')
  let droppedRules = $state<any>(null)
  let importError = $state('')

  // GeoSite categories for autocomplete
  let geositeCategories = $state([])

  // Routing test / dry-run
  let testUrl = $state('')
  let testing = $state(false)
  let testResult = $state(null)
  let testError = $state('')

  async function runTest() {
    if (!testUrl.trim()) return
    testing = true
    testResult = null
    testError = ''
    try {
      testResult = await api.testRouting(testUrl.trim())
    } catch (e) {
      testError = e.message
    } finally {
      testing = false
    }
  }

  function actionColor(action) {
    switch (action) {
      case 'proxy': return 'var(--accent)'
      case 'direct': return 'var(--accent-green)'
      case 'reject': return 'var(--accent-red)'
      default: return 'var(--text-secondary)'
    }
  }

  onMount(async () => {
    routing = await api.getRouting()
    // Normalize rules to have a 'type' field for the UI
    routing.rules = (routing.rules || []).map(normalizeRule)
    // Load templates and geosite categories
    try { templates = await api.getRoutingTemplates() } catch {}
    try { geositeCategories = await api.getGeositeCategories() } catch {}
  })

  function normalizeRule(rule) {
    if (rule._type) return rule
    if (rule.domains) return { _type: 'domain', value: rule.domains, action: rule.action }
    if (rule.geosite) return { _type: 'geosite', value: rule.geosite, action: rule.action }
    if (rule.process && rule.process.length) return { _type: 'process', value: rule.process.join(', '), action: rule.action }
    if (rule.geoip) return { _type: 'geoip', value: rule.geoip, action: rule.action }
    if (rule.ip_cidr && rule.ip_cidr.length) return { _type: 'ip_cidr', value: rule.ip_cidr.join(', '), action: rule.action }
    return { _type: 'domain', value: '', action: rule.action || 'direct' }
  }

  function toAPIRule(rule) {
    const out = { action: rule.action }
    switch (rule._type) {
      case 'domain': out.domains = rule.value; break
      case 'geosite': out.geosite = rule.value; break
      case 'process': out.process = rule.value.split(',').map(s => s.trim()).filter(Boolean); break
      case 'geoip': out.geoip = rule.value; break
      case 'ip_cidr': out.ip_cidr = rule.value.split(',').map(s => s.trim()).filter(Boolean); break
    }
    return out
  }

  function addRule() {
    routing.rules = [...routing.rules, { _type: 'domain', value: '', action: 'direct' }]
  }

  function removeRule(i) {
    routing.rules = routing.rules.filter((_, idx) => idx !== i)
  }

  async function openProcessPicker(index) {
    pickerTargetIndex = index
    msg = ''
    try {
      processes = await api.getProcesses()
      showProcessPicker = true
    } catch (e) {
      msg = 'Failed to load processes: ' + e.message
    }
  }

  function selectProcess(procName) {
    if (pickerTargetIndex < 0) return
    const rule = routing.rules[pickerTargetIndex]
    const existing = rule.value ? rule.value.split(',').map(s => s.trim()) : []
    if (!existing.includes(procName)) {
      existing.push(procName)
      routing.rules[pickerTargetIndex] = { ...rule, value: existing.join(', ') }
    }
  }

  function closeProcessPicker() {
    showProcessPicker = false
    pickerTargetIndex = -1
  }

  function ruleLabel(rule) {
    if (rule._type === 'process' && rule.value) {
      return rule.value.split(',').map(s => `[${s.trim()}]`).join(' ')
    }
    return rule.value
  }

  async function save() {
    saving = true
    msg = ''
    try {
      const apiRouting = {
        ...routing,
        rules: routing.rules.map(toAPIRule),
      }
      const res = await api.putRouting(apiRouting)
      msg = res.error || 'Saved'
    } finally {
      saving = false
    }
  }

  async function applyTemplate(id) {
    applyingTemplate = true
    try {
      await api.applyRoutingTemplate(id)
      // Reload rules after template applied
      routing = await api.getRouting()
      routing.rules = routing.rules.map(normalizeRule)
      showTemplates = false
      msg = 'Template applied'
    } catch (e) {
      msg = e.message
    } finally {
      applyingTemplate = false
    }
  }

  function validateRulesData(data: any): boolean {
    if (!data || typeof data !== 'object') return false
    if (!Array.isArray(data.rules)) return false
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
      let parsed: any
      if (droppedRules) {
        parsed = droppedRules
      } else if (importData.trim()) {
        parsed = JSON.parse(importData)
      } else {
        return
      }
      if (!validateRulesData(parsed)) {
        importError = 'Invalid file: expected JSON with a "rules" array'
        return
      }
      const result = await api.importRouting(parsed, importMode)
      // Reload rules
      routing = await api.getRouting()
      routing.rules = routing.rules.map(normalizeRule)
      showImport = false
      importData = ''
      droppedRules = null
      droppedFileName = ''
      importMode = 'merge'
      msg = `Imported ${result.added} rule(s)`
    } catch (e) {
      importError = 'Import failed: ' + e.message
    } finally {
      importing = false
    }
  }

  function closeImport() {
    showImport = false
    importData = ''
    droppedRules = null
    droppedFileName = ''
    importError = ''
    importMode = 'merge'
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
      msg = 'Rules exported'
    } catch (e) {
      msg = 'Export failed: ' + e.message
    }
  }
</script>

<div class="page">
  <div class="header">
    <h2>{t('routing.title')}</h2>
    <div class="header-actions">
      <button class="btn-template" onclick={() => (showTemplates = true)}>{t('routing.templates')}</button>
      <button class="btn-import" onclick={() => (showImport = true)}>{t('routing.import')}</button>
      <button class="btn-export" onclick={exportRules}>{t('routing.export')}</button>
    </div>
  </div>

  <div class="test-section">
    <span class="test-label">{t('routing.testUrl')}</span>
    <div class="test-row">
      <input
        class="test-input"
        bind:value={testUrl}
        placeholder={t('routing.testPlaceholder')}
        onkeydown={(e) => e.key === 'Enter' && runTest()}
      />
      <button class="test-btn" onclick={runTest} disabled={testing || !testUrl.trim()}>
        {testing ? t('routing.testing') : t('routing.test')}
      </button>
    </div>
    {#if testResult}
      <div class="test-result">
        <span class="test-result-action" style="color: {actionColor(testResult.action)}">
          {testResult.action.toUpperCase()}
        </span>
        <span class="test-result-detail">
          {t('routing.matchedBy')}: <strong>{testResult.matched_by}</strong>
          {#if testResult.rule}
            &mdash; {testResult.rule}
          {/if}
        </span>
      </div>
    {/if}
    {#if testError}
      <p class="test-error">{testError}</p>
    {/if}
  </div>

  <label class="default-row">
    <span>{t('routing.defaultAction')}</span>
    <select bind:value={routing.default}>
      <option value="proxy">{t('routing.proxy')}</option>
      <option value="direct">{t('routing.direct')}</option>
    </select>
  </label>

  <div class="rules">
    {#each routing.rules as rule, i}
      <div class="rule">
        <select bind:value={rule._type} class="type-select">
          <option value="domain">{t('routing.typeDomain')}</option>
          <option value="geosite">{t('routing.typeGeosite')}</option>
          <option value="process">{t('routing.typeProcess')}</option>
          <option value="geoip">{t('routing.typeGeoip')}</option>
          <option value="ip_cidr">{t('routing.typeIpCidr')}</option>
        </select>

        {#if rule._type === 'process'}
          <div class="process-field">
            <input bind:value={rule.value} placeholder="chrome.exe, WeChat.exe" />
            <button class="pick-btn" onclick={() => openProcessPicker(i)}>{t('routing.pick')}</button>
          </div>
        {:else if rule._type === 'geosite'}
          <input bind:value={rule.value} placeholder="category-ads, cn, geolocation-!cn" class="value-input" list="geosite-cats" />
        {:else if rule._type === 'domain'}
          <input bind:value={rule.value} placeholder="+.example.com, ads.example.com" class="value-input" />
        {:else if rule._type === 'geoip'}
          <input bind:value={rule.value} placeholder="CN" class="value-input" />
        {:else}
          <input bind:value={rule.value} placeholder="192.168.0.0/16, 10.0.0.0/8" class="value-input" />
        {/if}

        <select bind:value={rule.action}>
          <option value="direct">{t('routing.direct')}</option>
          <option value="proxy">{t('routing.proxy')}</option>
          <option value="reject">{t('routing.reject')}</option>
        </select>
        <button class="remove" onclick={() => removeRule(i)}>x</button>
      </div>
    {/each}
  </div>

  <div class="actions">
    <button class="add" onclick={addRule}>+ {t('routing.addRule')}</button>
    <button class="save" onclick={save} disabled={saving}>
      {saving ? t('routing.saving') : t('routing.saveApply')}
    </button>
  </div>
  {#if msg}<p class="msg">{msg}</p>{/if}

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
      <button class="btn-import-action" onclick={() => { if (droppedRules) { doImport() } else { showImport = true } }} disabled={importing}>
        {importing ? t('routing.importing') : t('routing.import')}
      </button>
      <button class="btn-export-action" onclick={exportRules}>
        {t('routing.export')}
      </button>
    </div>
  </div>
</div>

<datalist id="geosite-cats">
  {#each geositeCategories as cat}
    <option value={cat} />
  {/each}
</datalist>

{#if showTemplates}
<div class="overlay" onclick={() => (showTemplates = false)} role="dialog" aria-modal="true" aria-labelledby="templates-dialog-title" onkeydown={(e) => e.key === 'Escape' && (showTemplates = false)}>
  <div class="modal" onclick={(e) => e.stopPropagation()}>
    <h3 id="templates-dialog-title">{t('routing.routingTemplates')}</h3>
    <p class="modal-hint">{t('routing.templateHint')}</p>
    <div class="template-list">
      {#each templates as t}
        <button class="template-item" onclick={() => applyTemplate(t.id)} disabled={applyingTemplate}>
          <span class="template-name">{t.name}</span>
          <span class="template-desc">{t.description}</span>
        </button>
      {/each}
    </div>
    <button class="close-btn" onclick={() => (showTemplates = false)}>{t('common.cancel')}</button>
  </div>
</div>
{/if}

{#if showImport}
<div class="overlay" onclick={closeImport} role="dialog" aria-modal="true" aria-labelledby="import-rules-dialog-title" onkeydown={(e) => e.key === 'Escape' && closeImport()}>
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
      <button class="close-btn" onclick={closeImport}>{t('common.cancel')}</button>
      <button class="apply-btn" onclick={doImport} disabled={importing || !importData.trim()}>
        {importing ? t('routing.importing') : t('routing.import')}
      </button>
    </div>
  </div>
</div>
{/if}

{#if showProcessPicker}
<div class="overlay" onclick={closeProcessPicker} role="dialog" aria-modal="true" aria-labelledby="process-picker-dialog-title" onkeydown={(e) => e.key === 'Escape' && closeProcessPicker()}>
  <div class="picker" onclick={(e) => e.stopPropagation()}>
    <h3 id="process-picker-dialog-title">{t('routing.selectProcess')}</h3>
    <p class="picker-hint">{t('routing.selectProcessHint')}</p>
    {#if processes.length}
      <div class="proc-list">
        {#each processes as proc}
          <button class="proc-item" onclick={() => selectProcess(proc.name)}>
            <span class="proc-name">{proc.name}</span>
            <span class="proc-conns">{proc.conns} conn{proc.conns !== 1 ? 's' : ''}</span>
          </button>
        {/each}
      </div>
    {:else}
      <p class="empty">{t('routing.noProcesses')}</p>
    {/if}
    <button class="close-btn" onclick={closeProcessPicker}>{t('routing.done')}</button>
  </div>
</div>
{/if}

<style>
  .page { max-width: 700px; }

  .header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 20px;
  }

  .header h2 { margin: 0; font-size: 18px; }

  .header-actions {
    display: flex;
    gap: 8px;
  }

  .btn-template, .btn-import, .btn-export {
    background: var(--bg-tertiary);
    color: var(--text-secondary);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 6px 12px;
    cursor: pointer;
    font-size: 12px;
  }

  .btn-template:hover, .btn-import:hover, .btn-export:hover {
    background: #30363d;
    color: var(--text-primary);
  }

  .btn-template { color: var(--accent-purple); }
  .btn-import { color: var(--accent); }
  .btn-export { color: var(--accent-green); }

  /* Test URL section */
  .test-section {
    margin-bottom: 20px;
    padding: 12px;
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 8px;
  }

  .test-label {
    font-size: 13px;
    color: var(--text-secondary);
    display: block;
    margin-bottom: 8px;
  }

  .test-row {
    display: flex;
    gap: 8px;
  }

  .test-input {
    flex: 1;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px 12px;
    color: var(--text-primary);
    font-size: 13px;
  }

  .test-input:focus { outline: none; border-color: var(--accent); }

  .test-btn {
    background: var(--bg-tertiary);
    color: var(--accent);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px 16px;
    cursor: pointer;
    font-size: 13px;
    white-space: nowrap;
  }

  .test-btn:hover { background: #30363d; }
  .test-btn:disabled { opacity: 0.5; cursor: default; }

  .test-result {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-top: 10px;
    padding: 8px 12px;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    font-size: 13px;
  }

  .test-result-action {
    font-weight: 600;
    font-size: 12px;
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }

  .test-result-detail {
    color: var(--text-secondary);
  }

  .test-result-detail strong {
    color: var(--text-primary);
  }

  .test-error {
    font-size: 12px;
    color: var(--accent-red);
    margin: 8px 0 0;
  }

  .default-row {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 16px;
  }

  .default-row span { font-size: 13px; color: var(--text-secondary); }

  select {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 6px 10px;
    color: var(--text-primary);
    font-size: 13px;
  }

  .rules { display: flex; flex-direction: column; gap: 8px; }

  .rule {
    display: flex;
    gap: 8px;
    align-items: center;
  }

  .type-select { min-width: 100px; }

  .value-input, .process-field input {
    flex: 1;
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px 12px;
    color: var(--text-primary);
    font-size: 13px;
  }

  .value-input:focus, .process-field input:focus { outline: none; border-color: var(--accent); }

  .process-field {
    flex: 1;
    display: flex;
    gap: 4px;
  }

  .pick-btn {
    background: var(--bg-tertiary);
    color: var(--accent);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 6px 12px;
    cursor: pointer;
    font-size: 12px;
    white-space: nowrap;
  }

  .pick-btn:hover { background: #30363d; }

  .remove {
    background: none;
    border: 1px solid var(--border);
    color: var(--accent-red);
    border-radius: 6px;
    padding: 6px 10px;
    cursor: pointer;
  }

  .actions {
    display: flex;
    gap: 8px;
    margin-top: 16px;
  }

  .add {
    background: var(--bg-tertiary);
    color: var(--text-primary);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px 16px;
    cursor: pointer;
    font-size: 13px;
  }

  .save {
    background: var(--btn-bg);
    color: #fff;
    border: none;
    border-radius: 6px;
    padding: 8px 16px;
    cursor: pointer;
    font-size: 13px;
  }

  .save:disabled { opacity: 0.5; }
  .msg { font-size: 13px; color: var(--text-secondary); margin-top: 8px; }

  /* Process picker overlay */
  .overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
  }

  .picker {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 20px;
    width: 400px;
    max-height: 500px;
    display: flex;
    flex-direction: column;
  }

  .picker h3 { font-size: 16px; margin: 0 0 4px; color: var(--text-primary); }
  .picker-hint { font-size: 12px; color: var(--text-muted); margin: 0 0 12px; }

  .proc-list {
    overflow-y: auto;
    max-height: 350px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .proc-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px 12px;
    cursor: pointer;
    color: var(--text-primary);
    font-size: 13px;
    width: 100%;
    text-align: left;
  }

  .proc-item:hover { border-color: var(--accent); }
  .proc-name { font-weight: 500; }
  .proc-conns { font-size: 11px; color: var(--text-muted); }

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

  .empty { font-size: 13px; color: var(--text-muted); }

  /* Modal styles */
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

  .template-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
    overflow-y: auto;
    max-height: 300px;
  }

  .template-item {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 12px;
    cursor: pointer;
    color: var(--text-primary);
    text-align: left;
    width: 100%;
  }

  .template-item:hover { border-color: var(--accent-purple); }
  .template-item:disabled { opacity: 0.5; cursor: default; }

  .template-name { font-weight: 500; font-size: 14px; }
  .template-desc { font-size: 12px; color: var(--text-secondary); margin-top: 4px; }

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

  /* Import / Export section */
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
</style>
