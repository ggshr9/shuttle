<script lang="ts">
  import { t } from '../i18n/index'

  let {
    rules = $bindable(),
    geositeCategories,
    onOpenProcessPicker,
  } = $props()

  function addRule() {
    rules = [...rules, { _type: 'domain', value: '', action: 'direct' }]
  }

  function removeRule(i) {
    rules = rules.filter((_, idx) => idx !== i)
  }
</script>

<div class="rules">
  {#each rules as rule, i}
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
          <button class="pick-btn" onclick={() => onOpenProcessPicker(i)}>{t('routing.pick')}</button>
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

<datalist id="geosite-cats">
  {#each geositeCategories as cat}
    <option value={cat} />
  {/each}
</datalist>

<div class="actions">
  <button class="add" onclick={addRule}>+ {t('routing.addRule')}</button>
</div>

<style>
  .rules { display: flex; flex-direction: column; gap: 8px; }

  .rule {
    display: flex;
    gap: 8px;
    align-items: center;
  }

  .type-select { min-width: 100px; }

  select {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 6px 10px;
    color: var(--text-primary);
    font-size: 13px;
  }

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
</style>
