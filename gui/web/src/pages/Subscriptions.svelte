<script lang="ts">
  import { api } from '../lib/api'
  import { onMount } from 'svelte'
  import { toast } from '../lib/toast'
  import { t } from '../lib/i18n/index'

  let subscriptions = $state([])
  let loading = $state(true)

  // Add subscription dialog
  let showAdd = $state(false)
  let newSub = $state({ name: '', url: '' })
  let adding = $state(false)

  // Refreshing state
  let refreshing = $state({})

  onMount(async () => {
    await loadSubscriptions()
  })

  async function loadSubscriptions() {
    loading = true
    try {
      subscriptions = await api.getSubscriptions()
    } catch (e) {
      toast.error('Failed to load subscriptions: ' + (e as Error).message)
    } finally {
      loading = false
    }
  }

  async function addSubscription() {
    if (!newSub.url.trim()) return
    adding = true
    try {
      const sub = await api.addSubscription(newSub.name, newSub.url)
      subscriptions = [...subscriptions, sub]
      newSub = { name: '', url: '' }
      showAdd = false
      toast.success(`Added subscription with ${sub.servers?.length || 0} servers`)
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      adding = false
    }
  }

  async function refreshSubscription(id) {
    refreshing[id] = true
    refreshing = { ...refreshing }
    try {
      const updated = await api.refreshSubscription(id)
      subscriptions = subscriptions.map(s => s.id === id ? updated : s)
      toast.success(`Refreshed: ${updated.servers?.length || 0} servers`)
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      refreshing[id] = false
      refreshing = { ...refreshing }
    }
  }

  async function deleteSubscription(id) {
    try {
      await api.deleteSubscription(id)
      subscriptions = subscriptions.filter(s => s.id !== id)
      toast.success('Subscription deleted')
    } catch (e) {
      toast.error((e as Error).message)
    }
  }

  function formatDate(dateStr) {
    if (!dateStr) return t('subscriptions.never')
    const date = new Date(dateStr)
    return date.toLocaleString()
  }
</script>

<div class="page">
  <div class="page-header">
    <h2>{t('subscriptions.title')}</h2>
    <button class="btn-primary" onclick={() => (showAdd = true)}>{t('subscriptions.add')}</button>
  </div>

  {#if loading}
    <p class="loading-text">{t('common.loading')}</p>
  {:else if subscriptions.length === 0}
    <div class="empty">
      <svg width="48" height="48" viewBox="0 0 48 48" fill="none" stroke="var(--text-muted)" stroke-width="1.5">
        <path d="M10 14h28M10 24h28M10 34h20"/>
        <circle cx="38" cy="34" r="5"/>
      </svg>
      <p>{t('subscriptions.noSubscriptions')}</p>
      <p class="help">{t('subscriptions.noSubscriptionsHelp')}</p>
    </div>
  {:else}
    <div class="sub-list">
      {#each subscriptions as sub}
        <div class="sub-card" class:has-error={sub.error}>
          <div class="sub-info">
            <div class="sub-header">
              <span class="sub-name">{sub.name || 'Unnamed'}</span>
              <span class="sub-count">{t('subscriptions.servers', { count: sub.servers?.length || 0 })}</span>
            </div>
            <div class="sub-url">{sub.url}</div>
            <div class="sub-meta">
              {t('subscriptions.updated', { date: formatDate(sub.updated_at) })}
              {#if sub.error}
                <span class="sub-error">{t('subscriptions.error', { message: sub.error })}</span>
              {/if}
            </div>
          </div>
          <div class="sub-actions">
            <button
              class="btn-sm"
              onclick={() => refreshSubscription(sub.id)}
              disabled={refreshing[sub.id]}
            >
              {refreshing[sub.id] ? t('subscriptions.refreshing') : t('subscriptions.refresh')}
            </button>
            <button
              class="btn-sm danger"
              onclick={() => deleteSubscription(sub.id)}
            >
              {t('subscriptions.delete')}
            </button>
          </div>
        </div>

        {#if sub.servers?.length > 0}
          <div class="servers-preview">
            {#each sub.servers.slice(0, 5) as srv}
              <div class="server-preview">
                <span class="srv-name">{srv.name || srv.addr}</span>
                <span class="srv-addr">{srv.addr}</span>
              </div>
            {/each}
            {#if sub.servers.length > 5}
              <div class="more-servers">{t('subscriptions.moreServers', { count: sub.servers.length - 5 })}</div>
            {/if}
          </div>
        {/if}
      {/each}
    </div>
  {/if}
</div>

{#if showAdd}
  <div
    class="modal-overlay"
    role="dialog"
    aria-modal="true"
    aria-labelledby="add-sub-dialog-title"
    onclick={() => (showAdd = false)}
    onkeydown={(e) => e.key === 'Escape' && (showAdd = false)}
  >
    <div class="modal" onclick={(e) => e.stopPropagation()}>
      <div class="modal-header">
        <h3 id="add-sub-dialog-title">{t('subscriptions.add')}</h3>
        <button class="modal-close" onclick={() => (showAdd = false)}>&times;</button>
      </div>
      <div class="modal-body">
        <label>
          <span>{t('subscriptions.name')}</span>
          <input bind:value={newSub.name} placeholder="My Subscription" />
        </label>
        <label>
          <span>{t('subscriptions.url')}</span>
          <input bind:value={newSub.url} placeholder="https://example.com/subscribe" />
        </label>
        <p class="help-text">
          {t('subscriptions.supportedFormats')}
        </p>
      </div>
      <div class="modal-footer">
        <button class="btn-cancel" onclick={() => (showAdd = false)}>{t('common.cancel')}</button>
        <button
          class="btn-primary"
          onclick={addSubscription}
          disabled={adding || !newSub.url.trim()}
        >
          {adding ? t('subscriptions.adding') : t('subscriptions.addBtn')}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  .page { max-width: 740px; }

  .page-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 20px;
  }

  h2 { font-size: 18px; font-weight: 600; margin: 0; letter-spacing: -0.01em; }
  h3 { font-size: 14px; font-weight: 600; color: var(--text-primary); margin: 0; }

  .btn-primary {
    background: var(--btn-bg);
    color: #fff;
    border: none;
    border-radius: var(--radius-sm);
    padding: 8px 16px;
    cursor: pointer;
    font-size: 13px;
    font-weight: 500;
    font-family: inherit;
    transition: background 0.15s;
  }
  .btn-primary:hover { background: var(--btn-bg-hover); }
  .btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }

  .loading-text { color: var(--text-secondary); font-size: 14px; }

  .empty {
    text-align: center;
    padding: 48px;
    color: var(--text-secondary);
    background: var(--bg-secondary);
    border: 1px dashed var(--border);
    border-radius: var(--radius-lg);
  }
  .empty p { margin: 12px 0 0; }
  .empty .help { font-size: 13px; color: var(--text-muted); }

  .sub-list { display: flex; flex-direction: column; gap: 10px; }

  .sub-card {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: 16px 20px;
    transition: border-color 0.15s;
  }
  .sub-card:hover { border-color: var(--border-light); }
  .sub-card.has-error { border-color: var(--accent-red); }

  .sub-info { flex: 1; min-width: 0; }

  .sub-header {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 6px;
  }

  .sub-name {
    font-size: 15px;
    font-weight: 600;
    color: var(--text-primary);
  }

  .sub-count {
    font-size: 11px;
    color: var(--accent-green);
    background: var(--accent-green-subtle);
    padding: 2px 10px;
    border-radius: 10px;
    font-weight: 500;
  }

  .sub-url {
    font-size: 12px;
    color: var(--text-muted);
    word-break: break-all;
    margin-bottom: 6px;
    font-family: 'JetBrains Mono', monospace;
  }

  .sub-meta {
    font-size: 11px;
    color: var(--text-muted);
  }

  .sub-error {
    color: var(--accent-red);
    display: block;
    margin-top: 4px;
  }

  .sub-actions {
    display: flex;
    gap: 6px;
    margin-left: 16px;
    flex-shrink: 0;
  }

  .btn-sm {
    background: var(--bg-tertiary);
    color: var(--text-secondary);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 6px 12px;
    cursor: pointer;
    font-size: 12px;
    font-weight: 500;
    font-family: inherit;
    transition: all 0.15s;
  }
  .btn-sm:hover { background: var(--bg-hover); color: var(--text-primary); }
  .btn-sm:disabled { opacity: 0.5; cursor: default; }
  .btn-sm.danger { color: var(--accent-red); }
  .btn-sm.danger:hover { background: var(--accent-red-subtle); }

  .servers-preview {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 8px 14px;
    margin-top: -6px;
    margin-left: 20px;
    margin-right: 20px;
    margin-bottom: 4px;
  }

  .server-preview {
    display: flex;
    justify-content: space-between;
    padding: 5px 0;
    font-size: 12px;
    border-bottom: 1px solid var(--border);
  }
  .server-preview:last-child { border-bottom: none; }

  .srv-name { color: var(--text-primary); font-weight: 500; }
  .srv-addr { color: var(--text-muted); font-family: 'JetBrains Mono', monospace; }

  .more-servers {
    font-size: 11px;
    color: var(--text-muted);
    text-align: center;
    padding-top: 4px;
  }

  /* ===== Modal ===== */
  .modal-overlay {
    position: fixed;
    top: 0; left: 0; right: 0; bottom: 0;
    background: var(--overlay-bg);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
    backdrop-filter: blur(4px);
  }

  .modal {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    width: 90%;
    max-width: 440px;
    box-shadow: var(--shadow-lg);
  }

  .modal-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 18px 20px;
    border-bottom: 1px solid var(--border);
  }

  .modal-close {
    background: none;
    border: none;
    color: var(--text-muted);
    font-size: 22px;
    cursor: pointer;
    padding: 0;
    line-height: 1;
  }
  .modal-close:hover { color: var(--text-primary); }

  .modal-body {
    padding: 20px;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  .modal-body label {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .modal-body label span {
    font-size: 12px;
    color: var(--text-secondary);
    font-weight: 500;
  }

  .modal-body input {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 9px 12px;
    color: var(--text-primary);
    font-size: 14px;
    font-family: inherit;
  }

  .modal-body input:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-subtle);
  }

  .help-text {
    font-size: 12px;
    color: var(--text-muted);
    margin: 0;
  }

  .modal-footer {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    padding: 14px 20px;
    border-top: 1px solid var(--border);
  }

  .btn-cancel {
    background: var(--bg-tertiary);
    color: var(--text-primary);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 8px 16px;
    cursor: pointer;
    font-size: 13px;
    font-weight: 500;
    font-family: inherit;
  }
  .btn-cancel:hover { background: var(--bg-hover); }
</style>
