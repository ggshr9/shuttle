<script lang="ts">
  import { Button, Select, Input, Combobox } from '@/ui'
  import { getGeositeCategories } from '@/lib/api/endpoints'
  import { t } from '@/lib/i18n/index'
  import type { RoutingRule } from '@/lib/api/types'

  type RuleType = 'domain' | 'ip_cidr' | 'geoip' | 'geosite' | 'process'
  type Action = 'proxy' | 'direct' | 'reject'

  interface Props {
    initial: RoutingRule
    onSave: (r: RoutingRule) => void
    onCancel: () => void
  }
  let { initial, onSave, onCancel }: Props = $props()

  function detectType(r: RoutingRule): RuleType {
    if (r.domain !== undefined)  return 'domain'
    if (r.ip_cidr !== undefined) return 'ip_cidr'
    if (r.geoip !== undefined)   return 'geoip'
    if (r.geosite !== undefined) return 'geosite'
    if (r.process !== undefined) return 'process'
    return 'domain'
  }
  function initialValue(r: RoutingRule): string {
    return r.domain ?? r.ip_cidr ?? r.geoip ?? r.geosite ?? r.process ?? ''
  }

  let type = $state<RuleType>(detectType(initial))
  let value = $state(initialValue(initial))
  let action = $state<Action>((initial.action as Action) || 'proxy')

  let geositeOptions = $state<string[]>([])
  $effect(() => {
    if (type === 'geosite' && geositeOptions.length === 0) {
      getGeositeCategories().then((list) => { geositeOptions = list }).catch(() => {})
    }
  })

  const typeOptions = $derived([
    { value: 'domain',  label: t('routing.typeDomain') },
    { value: 'ip_cidr', label: t('routing.typeIpCidr') },
    { value: 'geoip',   label: t('routing.typeGeoip') },
    { value: 'geosite', label: t('routing.typeGeosite') },
    { value: 'process', label: t('routing.typeProcess') },
  ])

  const actionOptions = $derived([
    { value: 'proxy',  label: t('routing.action.proxy') },
    { value: 'direct', label: t('routing.action.direct') },
    { value: 'reject', label: t('routing.action.reject') },
  ])

  function placeholderFor(t: RuleType): string {
    switch (t) {
      case 'ip_cidr': return '1.2.3.4/24'
      case 'geoip':   return 'cn'
      case 'geosite': return 'google'
      case 'process': return 'chrome.exe'
      default:        return 'example.com'
    }
  }

  function save() {
    const trimmed = value.trim()
    if (!trimmed) return
    const rule: RoutingRule = { action }
    rule[type] = trimmed
    onSave(rule)
  }
</script>

<form class="form" onsubmit={(e) => { e.preventDefault(); save() }}>
  <label class="field">
    <span>{t('routing.fieldType')}</span>
    <Select value={type} options={typeOptions as unknown as { value: RuleType; label: string }[]} onValueChange={(v) => (type = v)} />
  </label>

  <label class="field">
    <span>{t('routing.fieldValue')}</span>
    {#if type === 'geosite'}
      <Combobox
        value={value}
        items={geositeOptions.map((o) => ({ value: o, label: o }))}
        placeholder={placeholderFor(type)}
        onValueChange={(v) => (value = v ?? '')}
      />
    {:else}
      <Input value={value} placeholder={placeholderFor(type)} oninput={(e) => (value = (e.currentTarget as HTMLInputElement).value)} data-field="value" />
    {/if}
  </label>

  <label class="field">
    <span>{t('routing.fieldAction')}</span>
    <Select value={action} options={actionOptions as unknown as { value: Action; label: string }[]} onValueChange={(v) => (action = v)} />
  </label>

  <div class="actions">
    <Button variant="ghost" onclick={onCancel}>{t('common.cancel')}</Button>
    <Button variant="primary" type="submit">{t('common.save')}</Button>
  </div>
</form>

<style>
  .form {
    display: flex; flex-direction: column;
    gap: var(--shuttle-space-3);
    padding: var(--shuttle-space-4);
    max-width: 480px;
  }
  .field {
    display: flex; flex-direction: column;
    gap: var(--shuttle-space-1);
  }
  .field > span {
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-muted);
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }
  .actions {
    display: flex; gap: var(--shuttle-space-2);
    justify-content: flex-end;
    margin-top: var(--shuttle-space-2);
  }
</style>
