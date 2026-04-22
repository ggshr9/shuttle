<script lang="ts">
  import { t } from '@/lib/i18n/index'
  import type { RoutingRule } from '@/lib/api/types'

  interface Props { rules: RoutingRule[] }
  let { rules }: Props = $props()

  function weight(r: RoutingRule): number {
    if (r.geosite) return 3
    if (r.geoip) return 2
    if (r.domain) return 1
    if (r.process) return 1
    return 1
  }

  function color(action: string): string {
    switch (action) {
      case 'proxy':   return 'var(--shuttle-info)'
      case 'direct':  return 'var(--shuttle-success)'
      case 'reject':  return 'var(--shuttle-danger)'
      default:        return 'var(--shuttle-fg-muted)'
    }
  }

  function label(r: RoutingRule, i: number): string {
    const kind = r.geosite ? `geosite:${r.geosite}`
      : r.geoip ? `geoip:${r.geoip}`
      : r.domain ? `domain:${r.domain}`
      : r.process ? `proc:${r.process}`
      : 'fallthrough'
    return `#${i + 1} ${kind} → ${r.action}`
  }
</script>

<div class="bar" aria-label={t('routing.hitBar.label')}>
  {#each rules as r, i}
    <div
      class="seg"
      style="flex: {weight(r)}; background: {color(r.action)}"
      title={label(r, i)}
    ></div>
  {/each}
</div>
{#if rules.length === 0}
  <div class="empty">{t('routing.hitBar.empty')}</div>
{/if}

<style>
  .bar {
    display: flex; width: 100%; height: 10px;
    border-radius: var(--shuttle-radius-sm);
    overflow: hidden;
    background: var(--shuttle-bg-subtle);
    border: 1px solid var(--shuttle-border);
    margin-bottom: var(--shuttle-space-3);
  }
  .seg {
    transition: flex var(--shuttle-duration);
    cursor: help;
  }
  .seg:hover { filter: brightness(1.15); }
  .empty {
    height: 10px; width: 100%;
    margin-bottom: var(--shuttle-space-3);
    border: 1px dashed var(--shuttle-border);
    border-radius: var(--shuttle-radius-sm);
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-muted);
    display: flex; align-items: center; justify-content: center;
  }
</style>
