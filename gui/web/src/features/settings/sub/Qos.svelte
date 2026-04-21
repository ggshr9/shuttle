<script lang="ts">
  import { Button, Field, Icon, Input, Select, Switch } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import type { QosPriority, QosRule } from '@/lib/api/types'
  import { settings } from '../config.svelte'
  import PageHeader from '../PageHeader.svelte'

  const qos = $derived(settings.draft?.qos)

  const priorityOptions: { value: QosPriority; label: string }[] = [
    { value: 'critical', label: t('settings.qosPriorities.critical') },
    { value: 'high',     label: t('settings.qosPriorities.high') },
    { value: 'normal',   label: t('settings.qosPriorities.normal') },
    { value: 'bulk',     label: t('settings.qosPriorities.bulk') },
    { value: 'low',      label: t('settings.qosPriorities.low') },
  ]

  function addRule(): void {
    if (!qos) return
    const next: QosRule = { priority: 'normal', ports: [] }
    qos.rules = [...(qos.rules ?? []), next]
  }

  function removeRule(idx: number): void {
    if (!qos?.rules) return
    qos.rules = qos.rules.filter((_, i) => i !== idx)
  }

  function onPortsInput(rule: QosRule, raw: string): void {
    const parsed = raw.split(',')
      .map((p) => parseInt(p.trim(), 10))
      .filter((n) => !isNaN(n))
    rule.ports = parsed
  }
</script>

<PageHeader title={t('settings.qos')} />

{#if qos}
  <Field label={t('settings.qosEnabled')} hint={t('settings.qosEnabledHint')}>
    <Switch bind:checked={qos.enabled} />
  </Field>

  {#if qos.enabled}
    <div class="rules">
      <div class="rules-header">{t('settings.qosRules')}</div>
      {#if (qos.rules ?? []).length === 0}
        <p class="empty">{t('settings.qosNoRules')}</p>
      {:else}
        {#each qos.rules ?? [] as rule, i (i)}
          <div class="rule">
            <Select
              value={rule.priority}
              options={priorityOptions}
              onValueChange={(v) => { rule.priority = v as QosPriority }}
            />
            <Input
              placeholder={t('settings.qosPorts')}
              value={(rule.ports ?? []).join(', ')}
              onchange={(e) => onPortsInput(rule, (e.target as HTMLInputElement).value)}
            />
            <button type="button" class="remove" onclick={() => removeRule(i)} aria-label="Remove">
              <Icon name="x" size={12} />
            </button>
          </div>
        {/each}
      {/if}
      <Button variant="ghost" onclick={addRule}>
        <Icon name="plus" size={12} />
        {t('settings.qosAddRule')}
      </Button>
    </div>
  {/if}
{/if}

<style>
  .rules {
    display: flex;
    flex-direction: column;
    gap: var(--shuttle-space-2);
    padding: var(--shuttle-space-3) 0;
  }
  .rules-header {
    font-size: var(--shuttle-text-xs);
    font-weight: var(--shuttle-weight-semibold);
    color: var(--shuttle-fg-muted);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .rule {
    display: grid;
    grid-template-columns: 140px 1fr 28px;
    gap: var(--shuttle-space-2);
    align-items: center;
  }
  .remove {
    border: 1px solid var(--shuttle-border);
    background: transparent;
    color: var(--shuttle-fg-muted);
    border-radius: var(--shuttle-radius-sm);
    height: 28px;
    width: 28px;
    cursor: pointer;
  }
  .remove:hover { color: var(--shuttle-danger); border-color: var(--shuttle-danger); }
  .empty {
    font-size: var(--shuttle-text-sm);
    color: var(--shuttle-fg-muted);
    font-style: italic;
    margin: 0;
  }
</style>
