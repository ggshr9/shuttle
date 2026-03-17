<script lang="ts">
  import { t } from '../i18n/index'

  let { config = $bindable() } = $props()
</script>

<section>
  <h3>{t('settings.qos')}</h3>
  <div class="system-proxy-row" style="margin-top: 0; padding-top: 0; border-top: none;">
    <label class="system-proxy-label">
      <input type="checkbox" bind:checked={config.qos.enabled} />
      <span class="label-text">{t('settings.qosEnabled')}</span>
      <span class="hint">{t('settings.qosEnabledHint')}</span>
    </label>
  </div>
  {#if config.qos.enabled}
    <div class="qos-rules">
      <div class="qos-rules-header">
        <span class="qos-rules-title">{t('settings.qosRules')}</span>
      </div>
      {#if config.qos.rules?.length > 0}
        {#each config.qos.rules as rule, i}
          <div class="qos-rule">
            <select bind:value={rule.priority} class="qos-priority-select">
              <option value="critical">{t('settings.qosPriorities.critical')}</option>
              <option value="high">{t('settings.qosPriorities.high')}</option>
              <option value="normal">{t('settings.qosPriorities.normal')}</option>
              <option value="bulk">{t('settings.qosPriorities.bulk')}</option>
              <option value="low">{t('settings.qosPriorities.low')}</option>
            </select>
            <input
              type="text"
              placeholder={t('settings.qosPorts')}
              value={rule.ports?.join(', ') || ''}
              onchange={(e) => {
                const ports = e.target.value.split(',').map(p => parseInt(p.trim())).filter(p => !isNaN(p))
                rule.ports = ports.length > 0 ? ports : undefined
              }}
              class="qos-input"
            />
            <button class="qos-remove" onclick={() => config.qos.rules = config.qos.rules.filter((_, j) => j !== i)}>×</button>
          </div>
        {/each}
      {:else}
        <p class="qos-no-rules">{t('settings.qosNoRules')}</p>
      {/if}
      <button class="qos-add" onclick={() => config.qos.rules = [...(config.qos.rules || []), { priority: 'normal', ports: [] }]}>
        + {t('settings.qosAddRule')}
      </button>
    </div>
  {/if}
</section>

<style>
  section {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 12px 16px;
    margin-bottom: 12px;
  }

  h3 { font-size: 14px; color: var(--text-secondary); margin: 20px 0 10px; }

  .system-proxy-row {
    margin-top: 12px;
    padding-top: 12px;
    border-top: 1px solid var(--border);
  }

  .system-proxy-label {
    display: flex;
    align-items: center;
    gap: 8px;
    cursor: pointer;
  }

  .system-proxy-label .label-text {
    font-size: 13px;
    color: var(--text-primary);
  }

  .system-proxy-label .hint {
    font-size: 11px;
    color: var(--text-secondary);
    margin-left: auto;
  }

  .qos-rules {
    margin-top: 12px;
  }

  .qos-rules-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 8px;
  }

  .qos-rules-title {
    font-size: 12px;
    color: var(--text-secondary);
  }

  .qos-rule {
    display: flex;
    gap: 8px;
    align-items: center;
    margin-bottom: 8px;
  }

  .qos-priority-select {
    min-width: 140px;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 6px 10px;
    color: var(--text-primary);
    font-size: 12px;
  }

  .qos-input {
    flex: 1;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 6px 10px;
    color: var(--text-primary);
    font-size: 12px;
  }

  .qos-remove {
    background: transparent;
    border: 1px solid var(--accent-red);
    color: var(--accent-red);
    border-radius: 4px;
    width: 24px;
    height: 24px;
    cursor: pointer;
    font-size: 14px;
    line-height: 1;
  }

  .qos-remove:hover {
    background: rgba(248, 81, 73, 0.1);
  }

  .qos-no-rules {
    font-size: 12px;
    color: #6e7681;
    font-style: italic;
    margin: 8px 0;
  }

  .qos-add {
    background: transparent;
    border: 1px dashed var(--border);
    color: var(--text-secondary);
    border-radius: 6px;
    padding: 8px 12px;
    cursor: pointer;
    font-size: 12px;
    width: 100%;
    margin-top: 8px;
  }

  .qos-add:hover {
    border-color: var(--accent);
    color: var(--accent);
  }
</style>
