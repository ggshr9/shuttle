<script lang="ts">
  import { api } from '../lib/api'
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

  async function doImport() {
    if (!importData.trim()) return
    importing = true
    try {
      const parsed = JSON.parse(importData)
      const result = await api.importRouting(parsed)
      // Reload rules
      routing = await api.getRouting()
      routing.rules = routing.rules.map(normalizeRule)
      showImport = false
      importData = ''
      msg = `Imported ${result.added} rule(s)`
    } catch (e) {
      msg = 'Import failed: ' + e.message
    } finally {
      importing = false
    }
  }

  function exportRules() {
    window.open(api.exportRouting(), '_blank')
  }
</script>

<div class="page">
  <div class="header">
    <h2>Routing Rules</h2>
    <div class="header-actions">
      <button class="btn-template" onclick={() => (showTemplates = true)}>Templates</button>
      <button class="btn-import" onclick={() => (showImport = true)}>Import</button>
      <button class="btn-export" onclick={exportRules}>Export</button>
    </div>
  </div>

  <div class="test-section">
    <span class="test-label">Test URL</span>
    <div class="test-row">
      <input
        class="test-input"
        bind:value={testUrl}
        placeholder="Enter domain or URL to test..."
        onkeydown={(e) => e.key === 'Enter' && runTest()}
      />
      <button class="test-btn" onclick={runTest} disabled={testing || !testUrl.trim()}>
        {testing ? 'Testing...' : 'Test'}
      </button>
    </div>
    {#if testResult}
      <div class="test-result">
        <span class="test-result-action" style="color: {actionColor(testResult.action)}">
          {testResult.action.toUpperCase()}
        </span>
        <span class="test-result-detail">
          Matched by: <strong>{testResult.matched_by}</strong>
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
    <span>Default Action</span>
    <select bind:value={routing.default}>
      <option value="proxy">Proxy</option>
      <option value="direct">Direct</option>
    </select>
  </label>

  <div class="rules">
    {#each routing.rules as rule, i}
      <div class="rule">
        <select bind:value={rule._type} class="type-select">
          <option value="domain">Domain</option>
          <option value="geosite">GeoSite</option>
          <option value="process">Process</option>
          <option value="geoip">GeoIP</option>
          <option value="ip_cidr">IP CIDR</option>
        </select>

        {#if rule._type === 'process'}
          <div class="process-field">
            <input bind:value={rule.value} placeholder="chrome.exe, WeChat.exe" />
            <button class="pick-btn" onclick={() => openProcessPicker(i)}>Pick</button>
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
          <option value="direct">Direct</option>
          <option value="proxy">Proxy</option>
          <option value="reject">Reject</option>
        </select>
        <button class="remove" onclick={() => removeRule(i)}>x</button>
      </div>
    {/each}
  </div>

  <div class="actions">
    <button class="add" onclick={addRule}>+ Add Rule</button>
    <button class="save" onclick={save} disabled={saving}>
      {saving ? 'Saving...' : 'Save & Apply'}
    </button>
  </div>
  {#if msg}<p class="msg">{msg}</p>{/if}
</div>

<datalist id="geosite-cats">
  {#each geositeCategories as cat}
    <option value={cat} />
  {/each}
</datalist>

{#if showTemplates}
<div class="overlay" onclick={() => (showTemplates = false)} role="dialog" aria-modal="true" aria-labelledby="templates-dialog-title" onkeydown={(e) => e.key === 'Escape' && (showTemplates = false)}>
  <div class="modal" onclick={(e) => e.stopPropagation()}>
    <h3 id="templates-dialog-title">Routing Templates</h3>
    <p class="modal-hint">Choose a template to replace current rules</p>
    <div class="template-list">
      {#each templates as t}
        <button class="template-item" onclick={() => applyTemplate(t.id)} disabled={applyingTemplate}>
          <span class="template-name">{t.name}</span>
          <span class="template-desc">{t.description}</span>
        </button>
      {/each}
    </div>
    <button class="close-btn" onclick={() => (showTemplates = false)}>Cancel</button>
  </div>
</div>
{/if}

{#if showImport}
<div class="overlay" onclick={() => (showImport = false)} role="dialog" aria-modal="true" aria-labelledby="import-rules-dialog-title" onkeydown={(e) => e.key === 'Escape' && (showImport = false)}>
  <div class="modal" onclick={(e) => e.stopPropagation()}>
    <h3 id="import-rules-dialog-title">Import Rules</h3>
    <p class="modal-hint">Paste JSON rules configuration</p>
    <textarea
      bind:value={importData}
      placeholder={'{"rules": [{"geosite": "cn", "action": "direct"}], "default": "proxy"}'}
      rows="8"
    ></textarea>
    <div class="modal-actions">
      <button class="close-btn" onclick={() => (showImport = false)}>Cancel</button>
      <button class="apply-btn" onclick={doImport} disabled={importing || !importData.trim()}>
        {importing ? 'Importing...' : 'Import'}
      </button>
    </div>
  </div>
</div>
{/if}

{#if showProcessPicker}
<div class="overlay" onclick={closeProcessPicker} role="dialog" aria-modal="true" aria-labelledby="process-picker-dialog-title" onkeydown={(e) => e.key === 'Escape' && closeProcessPicker()}>
  <div class="picker" onclick={(e) => e.stopPropagation()}>
    <h3 id="process-picker-dialog-title">Select Process</h3>
    <p class="picker-hint">Click a process to add it to the rule</p>
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
      <p class="empty">No processes with active connections found</p>
    {/if}
    <button class="close-btn" onclick={closeProcessPicker}>Done</button>
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
</style>
