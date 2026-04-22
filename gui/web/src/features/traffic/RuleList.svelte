<script lang="ts">
  import { Button, Icon } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { useCategories } from './resource.svelte'
  import type { RoutingRule } from '@/lib/api/types'

  // The shape the legacy component worked with — backend uses
  // {domain, geosite, geoip, process} as separate fields, but the editor
  // holds them in a unified `_type/value` pair and normalizes on save.
  interface UiRule extends RoutingRule {
    _type?: 'domain' | 'geosite' | 'process' | 'geoip' | 'ip_cidr'
    value?: string
  }

  interface Props {
    rules: RoutingRule[]
    onChange: (rules: RoutingRule[]) => void
    onOpenProcessPicker?: (idx: number) => void
  }
  let { rules, onChange, onOpenProcessPicker }: Props = $props()

  const cats = useCategories()

  // Derive UI state from the canonical rules shape (one field set at a time).
  function toUi(r: RoutingRule): UiRule {
    if (r.geosite) return { ...r, _type: 'geosite', value: r.geosite }
    if (r.geoip)   return { ...r, _type: 'geoip',   value: r.geoip }
    if (r.process) return { ...r, _type: 'process', value: r.process }
    if (r.ip_cidr) return { ...r, _type: 'ip_cidr', value: r.ip_cidr }
    if (r.domain)  return { ...r, _type: 'domain',  value: r.domain }
    return { ...r, _type: 'domain', value: '' }
  }

  function fromUi(u: UiRule): RoutingRule {
    const base = { action: u.action }
    if (u._type === 'geosite') return { ...base, geosite: u.value }
    if (u._type === 'geoip')   return { ...base, geoip:   u.value }
    if (u._type === 'process') return { ...base, process: u.value }
    if (u._type === 'ip_cidr') return { ...base, ip_cidr: u.value }
    return { ...base, domain: u.value }
  }

  function updateAt(i: number, next: UiRule) {
    const ui = rules.map(toUi)
    ui[i] = next
    onChange(ui.map(fromUi))
  }

  function addRule() {
    onChange([...rules, { domain: '', action: 'direct' }])
  }

  function removeRule(i: number) {
    onChange(rules.filter((_, idx) => idx !== i))
  }
</script>

<div class="rules">
  {#each rules as r, i}
    {@const ui = toUi(r)}
    <div class="rule">
      <select
        class="type-select"
        value={ui._type}
        onchange={(e) => updateAt(i, { ...ui, _type: (e.target as HTMLSelectElement).value as UiRule['_type'] })}
      >
        <option value="domain">{t('routing.typeDomain')}</option>
        <option value="geosite">{t('routing.typeGeosite')}</option>
        <option value="process">{t('routing.typeProcess')}</option>
        <option value="geoip">{t('routing.typeGeoip')}</option>
        <option value="ip_cidr">{t('routing.typeIpCidr')}</option>
      </select>

      {#if ui._type === 'process'}
        <div class="process-field">
          <input
            value={ui.value}
            placeholder="chrome.exe, WeChat.exe"
            oninput={(e) => updateAt(i, { ...ui, value: (e.target as HTMLInputElement).value })}
          />
          {#if onOpenProcessPicker}
            <Button size="sm" variant="ghost" onclick={() => onOpenProcessPicker(i)}>
              {t('routing.pick')}
            </Button>
          {/if}
        </div>
      {:else if ui._type === 'geosite'}
        <input
          class="value-input"
          value={ui.value}
          list="geosite-cats"
          placeholder="category-ads, cn, geolocation-!cn"
          oninput={(e) => updateAt(i, { ...ui, value: (e.target as HTMLInputElement).value })}
        />
      {:else if ui._type === 'domain'}
        <input
          class="value-input"
          value={ui.value}
          placeholder="+.example.com, ads.example.com"
          oninput={(e) => updateAt(i, { ...ui, value: (e.target as HTMLInputElement).value })}
        />
      {:else if ui._type === 'geoip'}
        <input
          class="value-input"
          value={ui.value}
          placeholder="CN"
          oninput={(e) => updateAt(i, { ...ui, value: (e.target as HTMLInputElement).value })}
        />
      {:else}
        <input
          class="value-input"
          value={ui.value}
          placeholder="192.168.0.0/16, 10.0.0.0/8"
          oninput={(e) => updateAt(i, { ...ui, value: (e.target as HTMLInputElement).value })}
        />
      {/if}

      <select
        value={ui.action}
        onchange={(e) => updateAt(i, { ...ui, action: (e.target as HTMLSelectElement).value })}
      >
        <option value="direct">{t('routing.direct')}</option>
        <option value="proxy">{t('routing.proxy')}</option>
        <option value="reject">{t('routing.reject')}</option>
      </select>

      <Button size="sm" variant="ghost" onclick={() => removeRule(i)}>
        <Icon name="trash" size={14} title={t('common.delete')} />
      </Button>
    </div>
  {/each}
</div>

<datalist id="geosite-cats">
  {#each cats.data ?? [] as cat}
    <option value={cat}></option>
  {/each}
</datalist>

<div class="actions">
  <Button variant="secondary" onclick={addRule}>
    <Icon name="plus" size={14} /> {t('routing.addRule')}
  </Button>
</div>

<style>
  .rules { display: flex; flex-direction: column; gap: var(--shuttle-space-2); }

  .rule {
    display: flex;
    gap: var(--shuttle-space-2);
    align-items: center;
  }

  .type-select { min-width: 100px; }

  select {
    background: var(--shuttle-bg-surface);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-sm);
    padding: 6px 10px;
    color: var(--shuttle-fg-primary);
    font-size: var(--shuttle-text-sm);
    font-family: var(--shuttle-font-sans);
  }

  .value-input, .process-field input {
    flex: 1;
    background: var(--shuttle-bg-surface);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-sm);
    padding: 8px 12px;
    color: var(--shuttle-fg-primary);
    font-size: var(--shuttle-text-sm);
    font-family: var(--shuttle-font-sans);
  }

  .value-input:focus, .process-field input:focus {
    outline: none;
    border-color: var(--shuttle-border-strong);
  }

  .process-field {
    flex: 1;
    display: flex;
    gap: var(--shuttle-space-1);
  }

  .actions {
    display: flex;
    gap: var(--shuttle-space-2);
    margin-top: var(--shuttle-space-4);
  }
</style>
