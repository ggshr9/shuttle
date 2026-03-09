<script lang="ts">
  import { api } from '../lib/api'
  import { onMount } from 'svelte'
  import { toast } from '../lib/toast'

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
    if (!dateStr) return 'Never'
    const date = new Date(dateStr)
    return date.toLocaleString()
  }
</script>

<div class="page">
  <div class="header">
    <h2>Subscriptions</h2>
    <button class="btn-add" onclick={() => (showAdd = true)}>Add Subscription</button>
  </div>

  {#if loading}
    <p class="loading">Loading...</p>
  {:else if subscriptions.length === 0}
    <div class="empty">
      <p>No subscriptions yet</p>
      <p class="help">Add a subscription URL to automatically import servers</p>
    </div>
  {:else}
    <div class="sub-list">
      {#each subscriptions as sub}
        <div class="sub-item" class:has-error={sub.error}>
          <div class="sub-info">
            <div class="sub-header">
              <span class="sub-name">{sub.name || 'Unnamed'}</span>
              <span class="sub-count">{sub.servers?.length || 0} servers</span>
            </div>
            <div class="sub-url">{sub.url}</div>
            <div class="sub-meta">
              Updated: {formatDate(sub.updated_at)}
              {#if sub.error}
                <span class="sub-error">Error: {sub.error}</span>
              {/if}
            </div>
          </div>
          <div class="sub-actions">
            <button
              class="btn-sm"
              onclick={() => refreshSubscription(sub.id)}
              disabled={refreshing[sub.id]}
            >
              {refreshing[sub.id] ? 'Refreshing...' : 'Refresh'}
            </button>
            <button
              class="btn-sm btn-danger"
              onclick={() => deleteSubscription(sub.id)}
            >
              Delete
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
              <div class="more-servers">+{sub.servers.length - 5} more</div>
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
        <h3 id="add-sub-dialog-title">Add Subscription</h3>
        <button class="modal-close" onclick={() => (showAdd = false)}>&times;</button>
      </div>
      <div class="modal-body">
        <label>
          <span>Name (optional)</span>
          <input bind:value={newSub.name} placeholder="My Subscription" />
        </label>
        <label>
          <span>Subscription URL</span>
          <input bind:value={newSub.url} placeholder="https://example.com/subscribe" />
        </label>
        <p class="help-text">
          Supported formats: Shuttle JSON, SIP008, Base64
        </p>
      </div>
      <div class="modal-footer">
        <button class="btn-cancel" onclick={() => (showAdd = false)}>Cancel</button>
        <button
          class="btn-primary"
          onclick={addSubscription}
          disabled={adding || !newSub.url.trim()}
        >
          {adding ? 'Adding...' : 'Add'}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  .page { max-width: 700px; }

  .header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 20px;
  }

  h2 { font-size: 18px; margin: 0; }

  .btn-add {
    background: #238636;
    color: #fff;
    border: none;
    border-radius: 6px;
    padding: 8px 16px;
    cursor: pointer;
    font-size: 13px;
  }
  .btn-add:hover { background: #2ea043; }

  .msg { font-size: 13px; color: #8b949e; margin-bottom: 12px; }
  .loading { color: #8b949e; }

  .empty {
    text-align: center;
    padding: 40px;
    color: #8b949e;
  }
  .empty p { margin: 8px 0; }
  .empty .help { font-size: 13px; color: #6e7681; }

  .sub-list { display: flex; flex-direction: column; gap: 12px; }

  .sub-item {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    background: #161b22;
    border: 1px solid #2d333b;
    border-radius: 8px;
    padding: 14px;
  }
  .sub-item.has-error { border-color: #f85149; }

  .sub-info { flex: 1; min-width: 0; }

  .sub-header {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 4px;
  }

  .sub-name {
    font-size: 15px;
    font-weight: 500;
    color: #e1e4e8;
  }

  .sub-count {
    font-size: 12px;
    color: #3fb950;
    background: rgba(63, 185, 80, 0.15);
    padding: 2px 8px;
    border-radius: 10px;
  }

  .sub-url {
    font-size: 12px;
    color: #8b949e;
    word-break: break-all;
    margin-bottom: 4px;
  }

  .sub-meta {
    font-size: 11px;
    color: #6e7681;
  }

  .sub-error {
    color: #f85149;
    display: block;
    margin-top: 4px;
  }

  .sub-actions {
    display: flex;
    gap: 6px;
    margin-left: 12px;
  }

  .btn-sm {
    background: #21262d;
    color: #e1e4e8;
    border: 1px solid #2d333b;
    border-radius: 6px;
    padding: 5px 12px;
    cursor: pointer;
    font-size: 12px;
  }
  .btn-sm:hover { background: #30363d; }
  .btn-sm:disabled { opacity: 0.5; cursor: default; }
  .btn-danger { color: #f85149; }
  .btn-danger:hover { background: #3d1f1f; }

  .servers-preview {
    background: #0d1117;
    border: 1px solid #21262d;
    border-radius: 6px;
    padding: 8px 12px;
    margin-top: -8px;
    margin-left: 16px;
    margin-right: 16px;
    margin-bottom: 4px;
  }

  .server-preview {
    display: flex;
    justify-content: space-between;
    padding: 4px 0;
    font-size: 12px;
    border-bottom: 1px solid #21262d;
  }
  .server-preview:last-child { border-bottom: none; }

  .srv-name { color: #e1e4e8; }
  .srv-addr { color: #6e7681; }

  .more-servers {
    font-size: 11px;
    color: #6e7681;
    text-align: center;
    padding-top: 4px;
  }

  /* Modal styles */
  .modal-overlay {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background: rgba(0, 0, 0, 0.7);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
  }

  .modal {
    background: #161b22;
    border: 1px solid #2d333b;
    border-radius: 12px;
    width: 90%;
    max-width: 440px;
  }

  .modal-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 16px;
    border-bottom: 1px solid #2d333b;
  }

  .modal-header h3 {
    margin: 0;
    font-size: 16px;
    color: #e1e4e8;
  }

  .modal-close {
    background: none;
    border: none;
    color: #8b949e;
    font-size: 24px;
    cursor: pointer;
    padding: 0;
    line-height: 1;
  }
  .modal-close:hover { color: #e1e4e8; }

  .modal-body {
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  .modal-body label {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .modal-body label span {
    font-size: 12px;
    color: #8b949e;
  }

  .modal-body input {
    background: #0d1117;
    border: 1px solid #2d333b;
    border-radius: 6px;
    padding: 8px 12px;
    color: #e1e4e8;
    font-size: 14px;
  }

  .modal-body input:focus {
    outline: none;
    border-color: #58a6ff;
  }

  .help-text {
    font-size: 12px;
    color: #6e7681;
    margin: 0;
  }

  .modal-footer {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    padding: 12px 16px;
    border-top: 1px solid #2d333b;
  }

  .btn-cancel {
    background: #21262d;
    color: #e1e4e8;
    border: 1px solid #2d333b;
    border-radius: 6px;
    padding: 8px 16px;
    cursor: pointer;
    font-size: 13px;
  }
  .btn-cancel:hover { background: #30363d; }

  .btn-primary {
    background: #238636;
    color: #fff;
    border: none;
    border-radius: 6px;
    padding: 8px 16px;
    cursor: pointer;
    font-size: 13px;
  }
  .btn-primary:hover { background: #2ea043; }
  .btn-primary:disabled { opacity: 0.5; }
</style>
