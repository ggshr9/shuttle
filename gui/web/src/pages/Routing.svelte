<script lang="ts">
  import { api } from '../lib/api'
  import { t } from '../lib/i18n/index'
  import { onMount } from 'svelte'

  import RoutingTestPanel from '../lib/routing/RoutingTestPanel.svelte'
  import RuleList from '../lib/routing/RuleList.svelte'
  import RoutingImportExport from '../lib/routing/RoutingImportExport.svelte'
  import RoutingTemplateModal from '../lib/routing/RoutingTemplateModal.svelte'
  import RoutingConfirmModal from '../lib/routing/RoutingConfirmModal.svelte'
  import RoutingProcessPicker from '../lib/routing/RoutingProcessPicker.svelte'

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
  let confirmTemplate = $state(null)

  // GeoSite categories for autocomplete
  let geositeCategories = $state([])

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

  onMount(async () => {
    routing = await api.getRouting()
    routing.rules = (routing.rules || []).map(normalizeRule)
    try { templates = await api.getRoutingTemplates() } catch {}
    try { geositeCategories = await api.getGeositeCategories() } catch {}
  })

  async function reloadRouting() {
    routing = await api.getRouting()
    routing.rules = routing.rules.map(normalizeRule)
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

  function requestApplyTemplate(id) {
    confirmTemplate = id
  }

  async function applyTemplate(id) {
    confirmTemplate = null
    applyingTemplate = true
    try {
      await api.applyRoutingTemplate(id)
      await reloadRouting()
      showTemplates = false
      msg = 'Template applied'
    } catch (e) {
      msg = e.message
    } finally {
      applyingTemplate = false
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
      msg = 'Rules exported'
    } catch (e) {
      msg = 'Export failed: ' + e.message
    }
  }

  function handleMessage(message: string) {
    msg = message
  }
</script>

<div class="page">
  <div class="header">
    <h2>{t('routing.title')}</h2>
    <div class="header-actions">
      <button class="btn-template" onclick={() => (showTemplates = true)}>{t('routing.templates')}</button>
      <button class="btn-import" onclick={() => {}}>{t('routing.import')}</button>
      <button class="btn-export" onclick={exportRules}>{t('routing.export')}</button>
    </div>
  </div>

  <RoutingTestPanel />

  <label class="default-row">
    <span>{t('routing.defaultAction')}</span>
    <select bind:value={routing.default}>
      <option value="proxy">{t('routing.proxy')}</option>
      <option value="direct">{t('routing.direct')}</option>
    </select>
  </label>

  <RuleList
    bind:rules={routing.rules}
    {geositeCategories}
    onOpenProcessPicker={openProcessPicker}
  />

  <div class="actions">
    <button class="save" onclick={save} disabled={saving}>
      {saving ? t('routing.saving') : t('routing.saveApply')}
    </button>
  </div>
  {#if msg}<p class="msg">{msg}</p>{/if}

  <RoutingImportExport
    onImportComplete={reloadRouting}
    onMessage={handleMessage}
  />
</div>

<RoutingTemplateModal
  bind:show={showTemplates}
  {templates}
  {applyingTemplate}
  onRequestApply={requestApplyTemplate}
/>

<RoutingConfirmModal
  bind:confirmTemplate
  {applyingTemplate}
  onApply={applyTemplate}
/>

<RoutingProcessPicker
  bind:show={showProcessPicker}
  {processes}
  onSelectProcess={selectProcess}
/>

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

  .actions {
    display: flex;
    gap: 8px;
    margin-top: 16px;
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
</style>
