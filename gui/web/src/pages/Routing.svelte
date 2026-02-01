<script>
  import { api } from '../lib/api.js'
  import { onMount } from 'svelte'

  let routing = $state({ rules: [], default: 'proxy', dns: {} })
  let saving = $state(false)
  let msg = $state('')

  onMount(async () => {
    routing = await api.getRouting()
  })

  function addRule() {
    routing.rules = [...routing.rules, { domains: '', action: 'direct' }]
  }

  function removeRule(i) {
    routing.rules = routing.rules.filter((_, idx) => idx !== i)
  }

  async function save() {
    saving = true
    msg = ''
    try {
      const res = await api.putRouting(routing)
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
        <input bind:value={rule.domains} placeholder="Domain pattern (e.g. geosite:cn)" />
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

<style>
  .page { max-width: 600px; }
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

  .rule input {
    flex: 1;
    background: #161b22;
    border: 1px solid #2d333b;
    border-radius: 6px;
    padding: 8px 12px;
    color: #e1e4e8;
    font-size: 13px;
  }

  .rule input:focus { outline: none; border-color: #58a6ff; }

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
</style>
