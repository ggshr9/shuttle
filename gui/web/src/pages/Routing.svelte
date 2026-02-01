<script>
  import { api } from '../lib/api.js'
  import { onMount } from 'svelte'

  let routing = $state({ rules: [], default: 'proxy', dns: {} })
  let saving = $state(false)
  let msg = $state('')
  let processes = $state([])
  let showProcessPicker = $state(false)
  let pickerTargetIndex = $state(-1)

  onMount(async () => {
    routing = await api.getRouting()
    // Normalize rules to have a 'type' field for the UI
    routing.rules = routing.rules.map(normalizeRule)
  })

  function normalizeRule(rule) {
    if (rule._type) return rule
    if (rule.domains) return { _type: 'domain', value: rule.domains, action: rule.action }
    if (rule.process && rule.process.length) return { _type: 'process', value: rule.process.join(', '), action: rule.action }
    if (rule.geoip) return { _type: 'geoip', value: rule.geoip, action: rule.action }
    if (rule.ip_cidr && rule.ip_cidr.length) return { _type: 'ip_cidr', value: rule.ip_cidr.join(', '), action: rule.action }
    return { _type: 'domain', value: '', action: rule.action || 'direct' }
  }

  function toAPIRule(rule) {
    const out = { action: rule.action }
    switch (rule._type) {
      case 'domain': out.domains = rule.value; break
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
</script>

<div class="page">
  <h2>Routing Rules</h2>

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
          <option value="process">Process</option>
          <option value="geoip">GeoIP</option>
          <option value="ip_cidr">IP CIDR</option>
        </select>

        {#if rule._type === 'process'}
          <div class="process-field">
            <input bind:value={rule.value} placeholder="chrome.exe, WeChat.exe" />
            <button class="pick-btn" onclick={() => openProcessPicker(i)}>Pick</button>
          </div>
        {:else if rule._type === 'domain'}
          <input bind:value={rule.value} placeholder="geosite:cn or *.example.com" class="value-input" />
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

{#if showProcessPicker}
<div class="overlay" onclick={closeProcessPicker}>
  <div class="picker" onclick={(e) => e.stopPropagation()}>
    <h3>Select Process</h3>
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
  h2 { font-size: 18px; margin-bottom: 20px; }

  .default-row {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 16px;
  }

  .default-row span { font-size: 13px; color: #8b949e; }

  select {
    background: #161b22;
    border: 1px solid #2d333b;
    border-radius: 6px;
    padding: 6px 10px;
    color: #e1e4e8;
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
    background: #161b22;
    border: 1px solid #2d333b;
    border-radius: 6px;
    padding: 8px 12px;
    color: #e1e4e8;
    font-size: 13px;
  }

  .value-input:focus, .process-field input:focus { outline: none; border-color: #58a6ff; }

  .process-field {
    flex: 1;
    display: flex;
    gap: 4px;
  }

  .pick-btn {
    background: #21262d;
    color: #58a6ff;
    border: 1px solid #2d333b;
    border-radius: 6px;
    padding: 6px 12px;
    cursor: pointer;
    font-size: 12px;
    white-space: nowrap;
  }

  .pick-btn:hover { background: #30363d; }

  .remove {
    background: none;
    border: 1px solid #2d333b;
    color: #f85149;
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
    background: #21262d;
    color: #e1e4e8;
    border: 1px solid #2d333b;
    border-radius: 6px;
    padding: 8px 16px;
    cursor: pointer;
    font-size: 13px;
  }

  .save {
    background: #238636;
    color: #fff;
    border: none;
    border-radius: 6px;
    padding: 8px 16px;
    cursor: pointer;
    font-size: 13px;
  }

  .save:disabled { opacity: 0.5; }
  .msg { font-size: 13px; color: #8b949e; margin-top: 8px; }

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
    background: #161b22;
    border: 1px solid #2d333b;
    border-radius: 12px;
    padding: 20px;
    width: 400px;
    max-height: 500px;
    display: flex;
    flex-direction: column;
  }

  .picker h3 { font-size: 16px; margin: 0 0 4px; color: #e1e4e8; }
  .picker-hint { font-size: 12px; color: #484f58; margin: 0 0 12px; }

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
    background: #0d1117;
    border: 1px solid #2d333b;
    border-radius: 6px;
    padding: 8px 12px;
    cursor: pointer;
    color: #e1e4e8;
    font-size: 13px;
    width: 100%;
    text-align: left;
  }

  .proc-item:hover { border-color: #58a6ff; }
  .proc-name { font-weight: 500; }
  .proc-conns { font-size: 11px; color: #484f58; }

  .close-btn {
    margin-top: 12px;
    background: #21262d;
    color: #e1e4e8;
    border: 1px solid #2d333b;
    border-radius: 6px;
    padding: 8px;
    cursor: pointer;
    font-size: 13px;
  }

  .empty { font-size: 13px; color: #484f58; }
</style>
