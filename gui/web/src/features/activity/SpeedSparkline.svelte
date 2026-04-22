<script lang="ts">
  import { Card } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { useSpeedHistory } from '@/lib/resources/status.svelte'

  const history = useSpeedHistory()

  const VIEW_W = 400
  const VIEW_H = 72

  function buildPath(samples: readonly number[]): string {
    if (samples.length === 0) return ''
    const max = Math.max(1, ...samples)
    const stepX = samples.length > 1 ? VIEW_W / (samples.length - 1) : 0
    return samples
      .map((v, i) => {
        const x = i * stepX
        const y = VIEW_H - (v / max) * (VIEW_H - 4)
        return `${i === 0 ? 'M' : 'L'} ${x.toFixed(1)} ${y.toFixed(1)}`
      })
      .join(' ')
  }

  function buildArea(samples: readonly number[]): string {
    const line = buildPath(samples)
    if (!line) return ''
    const stepX = samples.length > 1 ? VIEW_W / (samples.length - 1) : 0
    const lastX = ((samples.length - 1) * stepX).toFixed(1)
    return `${line} L ${lastX} ${VIEW_H} L 0 ${VIEW_H} Z`
  }

  const downPath = $derived(buildPath(history.down))
  const upPath   = $derived(buildPath(history.up))
  const downArea = $derived(buildArea(history.down))
</script>

<Card>
  <header>
    <h3>{t('dashboard.throughput.title')}</h3>
    <span class="legend">
      <span class="dot fg"></span>{t('dashboard.throughput.down')}
      <span class="dot mu"></span>{t('dashboard.throughput.up')}
      <span class="hint">{t('dashboard.throughput.window')}</span>
    </span>
  </header>
  <svg viewBox={`0 0 ${VIEW_W} ${VIEW_H}`} preserveAspectRatio="none" width="100%" height={VIEW_H}>
    {#if downArea}
      <path d={downArea} fill="currentColor" fill-opacity="0.08" />
      <path d={downPath} fill="none" stroke="currentColor" stroke-width="1.5" />
    {/if}
    {#if upPath}
      <path d={upPath} fill="none" stroke="var(--shuttle-fg-muted)" stroke-width="1" stroke-dasharray="2 2" />
    {/if}
  </svg>
  {#if history.down.length === 0}
    <div class="empty">{t('dashboard.throughput.waiting')}</div>
  {/if}
</Card>

<style>
  header { display: flex; align-items: center; gap: var(--shuttle-space-2); margin-bottom: var(--shuttle-space-2); }
  h3 { margin: 0; font-size: var(--shuttle-text-sm); font-weight: var(--shuttle-weight-semibold); color: var(--shuttle-fg-primary); }
  .legend {
    margin-left: auto; display: flex; align-items: center; gap: var(--shuttle-space-3);
    font-size: var(--shuttle-text-xs); color: var(--shuttle-fg-secondary);
  }
  .legend .dot { width: 8px; height: 2px; display: inline-block; margin-right: 4px; vertical-align: 1px; }
  .legend .fg { background: var(--shuttle-fg-primary); }
  .legend .mu { background: var(--shuttle-fg-muted); }
  .hint { color: var(--shuttle-fg-muted); }

  svg { color: var(--shuttle-fg-primary); display: block; }
  .empty {
    font-size: var(--shuttle-text-xs); color: var(--shuttle-fg-muted);
    text-align: center; margin-top: calc(-1 * var(--shuttle-space-6));
    padding-top: var(--shuttle-space-5);
  }
</style>
