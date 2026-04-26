<script lang="ts">
  import PageHeader from '../PageHeader.svelte'
  import { t } from '@/lib/i18n/index'
  import { getAdapter } from '@/lib/data'
  import type { DiagnosticsSnapshot } from '@/lib/data/diagnostics.svelte'

  const adapter = getAdapter()
  const snap = $derived<DiagnosticsSnapshot>(adapter.diagnostics.snapshot())

  const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })
  const dtf = new Intl.DateTimeFormat(undefined, { dateStyle: 'short', timeStyle: 'medium' })

  function relativeTime(at: number): string {
    const diff = Date.now() - Math.min(at, Date.now())
    if (diff < 60_000) return t('settings.diag.relativeJustNow')
    if (diff < 3_600_000) return rtf.format(-Math.floor(diff / 60_000), 'minute')
    if (diff < 86_400_000) return rtf.format(-Math.floor(diff / 3_600_000), 'hour')
    return rtf.format(-Math.floor(diff / 86_400_000), 'day')
  }

  function fmtPct(v: number): string {
    return `${(v * 100).toFixed(2)}%`
  }

  function fmtRtt(v: number | null): string {
    return v === null ? '—' : `${Math.round(v)} ${t('settings.diag.rttUnit')}`
  }

  function onResetClick() {
    if (confirm(t('settings.diag.action.confirmReset'))) {
      adapter.diagnostics.reset()
    }
  }
</script>

<PageHeader title={t('settings.diag.title')} />
<p class="subtitle">{t('settings.diag.subtitle')}</p>

<section class="block">
  <h3 class="head">{t('settings.diag.section.bridgeHealth')}</h3>
  <dl class="stats">
    <div><dt>{t('settings.diag.stat.requests')}</dt><dd>{snap.requestsTotal.toLocaleString()}</dd></div>
    <div>
      <dt>{t('settings.diag.stat.errors')}</dt>
      <dd>
        {snap.requestsErr.toLocaleString()}
        {#if snap.requestsTotal > 0}<span class="muted">({fmtPct(snap.errorRate)})</span>{/if}
      </dd>
    </div>
    <div>
      <dt>{t('settings.diag.stat.rttP50')}</dt>
      <dd>
        {#if snap.rttP50 === null}
          <span title={t('settings.diag.empty.noSamples')}>—</span>
        {:else}{fmtRtt(snap.rttP50)}{/if}
      </dd>
    </div>
    <div>
      <dt>{t('settings.diag.stat.rttP95')}</dt>
      <dd>
        {#if snap.rttP95 === null}
          <span title={t('settings.diag.empty.noSamples')}>—</span>
        {:else}{fmtRtt(snap.rttP95)}{/if}
      </dd>
    </div>
  </dl>
</section>

<section class="block">
  <h3 class="head">{t('settings.diag.section.lastError')}</h3>
  {#if snap.lastError}
    <div class="card">
      <code class="reason">{snap.lastError.reason}</code>
      <div class="when">{relativeTime(snap.lastError.at)}</div>
    </div>
  {:else}
    <div class="empty">{t('settings.diag.empty.noErrors')}</div>
  {/if}
</section>

<section class="block">
  <h3 class="head">{t('settings.diag.section.fallbackHistory')}</h3>
  {#if snap.fallbacks.length === 0}
    <div class="empty">{t('settings.diag.empty.noFallbacks')}</div>
  {:else}
    <ul class="list">
      {#each [...snap.fallbacks].reverse() as entry (entry.at + entry.reason)}
        <li>
          <code class="reason">{entry.reason}</code>
          <span class="when">{dtf.format(new Date(entry.at))}</span>
        </li>
      {/each}
    </ul>
    <div class="muted total">{t('settings.diag.totalTriggers').replace('{count}', String(snap.fallbacksTotal))}</div>
  {/if}
</section>

<section class="block">
  <button type="button" class="reset" onclick={onResetClick}>{t('settings.diag.action.reset')}</button>
</section>

<style>
  .subtitle {
    color: var(--shuttle-fg-muted);
    font-size: var(--shuttle-text-sm);
    margin: 0 0 var(--shuttle-space-5);
  }
  .block { margin-bottom: var(--shuttle-space-6); }
  .head {
    font-size: var(--shuttle-text-sm);
    font-weight: 600;
    color: var(--shuttle-fg-secondary);
    margin: 0 0 var(--shuttle-space-3);
  }
  .stats {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--shuttle-space-3);
    margin: 0;
  }
  .stats > div {
    background: var(--shuttle-bg-subtle);
    padding: var(--shuttle-space-3);
    border-radius: var(--shuttle-radius-md);
  }
  .stats dt { font-size: var(--shuttle-text-xs); color: var(--shuttle-fg-muted); margin-bottom: 2px; }
  .stats dd { font-size: var(--shuttle-text-lg); font-weight: 500; margin: 0; }
  .muted { color: var(--shuttle-fg-muted); font-size: var(--shuttle-text-xs); margin-left: var(--shuttle-space-2); }
  .total { margin: var(--shuttle-space-2) 0 0; }
  .card, .empty {
    background: var(--shuttle-bg-subtle);
    padding: var(--shuttle-space-3);
    border-radius: var(--shuttle-radius-md);
  }
  .reason {
    font-family: var(--shuttle-font-mono, ui-monospace, monospace);
    font-size: var(--shuttle-text-sm);
    word-break: break-all;
  }
  .when { color: var(--shuttle-fg-muted); font-size: var(--shuttle-text-xs); margin-top: 2px; }
  .empty { color: var(--shuttle-fg-muted); font-size: var(--shuttle-text-sm); }
  .list { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: var(--shuttle-space-2); }
  .list li {
    display: flex;
    justify-content: space-between;
    align-items: center;
    background: var(--shuttle-bg-subtle);
    padding: var(--shuttle-space-2) var(--shuttle-space-3);
    border-radius: var(--shuttle-radius-sm);
  }
  .reset {
    background: transparent;
    border: 1px solid var(--shuttle-border);
    color: var(--shuttle-fg-primary);
    padding: var(--shuttle-space-2) var(--shuttle-space-4);
    border-radius: var(--shuttle-radius-md);
    font-size: var(--shuttle-text-sm);
    cursor: pointer;
  }
  .reset:hover { background: var(--shuttle-bg-subtle); }
</style>
