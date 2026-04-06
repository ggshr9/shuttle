<script lang="ts">
  import { api } from '../lib/api'
  import { onMount } from 'svelte'
  import { toast } from '../lib/toast'
  import { t } from '../lib/i18n/index'

  interface GroupInfo {
    tag: string
    strategy: string
    members: string[]
    selected?: string
    latencies?: Record<string, number>
  }

  interface TestResult {
    tag: string
    latency_ms: number
    available: boolean
  }

  let groups = $state<GroupInfo[]>([])
  let loading = $state(true)
  let testing = $state<Record<string, boolean>>({})
  let testResults = $state<Record<string, TestResult[]>>({})
  let selecting = $state<Record<string, string>>({})

  onMount(async () => {
    await loadGroups()
  })

  async function loadGroups() {
    loading = true
    try {
      groups = await api.getGroups()
    } catch (e) {
      toast.error('Failed to load groups: ' + (e as Error).message)
      groups = []
    } finally {
      loading = false
    }
  }

  async function testGroup(tag: string) {
    testing = { ...testing, [tag]: true }
    try {
      const results = await api.testGroup(tag)
      testResults = { ...testResults, [tag]: results }
      toast.success(t('groups.testDone', { tag }))
      // Reload to get updated latencies
      await loadGroups()
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      testing = { ...testing, [tag]: false }
    }
  }

  async function selectMember(groupTag: string, memberTag: string) {
    selecting = { ...selecting, [groupTag]: memberTag }
    try {
      await api.selectGroupMember(groupTag, memberTag)
      await loadGroups()
      toast.success(t('groups.selectDone', { member: memberTag }))
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      const next = { ...selecting }
      delete next[groupTag]
      selecting = next
    }
  }

  function strategyColor(strategy: string): string {
    switch (strategy) {
      case 'failover':   return 'strategy-failover'
      case 'loadbalance': return 'strategy-loadbalance'
      case 'quality':    return 'strategy-quality'
      case 'url-test':   return 'strategy-urltest'
      case 'select':     return 'strategy-select'
      default:           return 'strategy-default'
    }
  }

  function formatLatency(ms: number): string {
    if (ms < 0) return t('groups.unreachable')
    return `${ms} ms`
  }

  function latencyClass(ms: number): string {
    if (ms < 0)    return 'lat-dead'
    if (ms < 100)  return 'lat-good'
    if (ms < 300)  return 'lat-ok'
    return 'lat-slow'
  }
</script>

<div class="page">
  <div class="page-header">
    <div class="page-title">
      <h2>{t('groups.title')}</h2>
      <span class="group-count">{groups.length} {t('groups.groups')}</span>
    </div>
    <button class="btn-sm" onclick={loadGroups} disabled={loading}>
      {loading ? t('common.loading') : t('groups.refresh')}
    </button>
  </div>

  {#if loading}
    <p class="loading-text">{t('common.loading')}</p>
  {:else if groups.length === 0}
    <div class="empty">
      <svg width="48" height="48" viewBox="0 0 48 48" fill="none" stroke="var(--text-muted)" stroke-width="1.5">
        <circle cx="24" cy="12" r="5"/>
        <circle cx="10" cy="36" r="5"/>
        <circle cx="38" cy="36" r="5"/>
        <path d="M24 17v6M24 23l-11 10M24 23l11 10"/>
      </svg>
      <p>{t('groups.noGroups')}</p>
      <p class="help">{t('groups.noGroupsHelp')}</p>
    </div>
  {:else}
    <div class="group-list">
      {#each groups as group (group.tag)}
        <div class="group-card">
          <div class="group-header">
            <div class="group-meta">
              <span class="group-tag">{group.tag}</span>
              <span class="strategy-badge {strategyColor(group.strategy)}">{group.strategy}</span>
              {#if group.selected}
                <span class="selected-badge">{t('groups.selected')}: {group.selected}</span>
              {/if}
            </div>
            <div class="group-actions">
              <button
                class="btn-sm"
                onclick={() => testGroup(group.tag)}
                disabled={testing[group.tag]}
              >
                {testing[group.tag] ? t('groups.testing') : t('groups.test')}
              </button>
            </div>
          </div>

          <div class="member-list">
            {#each group.members as member}
              {@const latency = group.latencies?.[member]}
              {@const isSelected = group.selected === member}
              {@const result = testResults[group.tag]?.find(r => r.tag === member)}
              <div class="member-row" class:is-selected={isSelected}>
                <div class="member-info">
                  <span class="member-tag">{member}</span>
                  {#if isSelected}
                    <span class="active-dot" title={t('groups.activeNode')}></span>
                  {/if}
                </div>
                <div class="member-right">
                  {#if result}
                    <span class="latency {latencyClass(result.latency_ms)}">{formatLatency(result.latency_ms)}</span>
                  {:else if latency !== undefined}
                    <span class="latency {latencyClass(latency)}">{formatLatency(latency)}</span>
                  {/if}
                  {#if group.strategy === 'select'}
                    <button
                      class="btn-select"
                      class:active={isSelected}
                      onclick={() => selectMember(group.tag, member)}
                      disabled={selecting[group.tag] !== undefined || isSelected}
                    >
                      {isSelected ? t('groups.active') : t('groups.use')}
                    </button>
                  {/if}
                </div>
              </div>
            {/each}

            {#if group.members.length === 0}
              <div class="no-members">{t('groups.noMembers')}</div>
            {/if}
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .page { max-width: 800px; }

  .page-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 20px;
  }

  .page-title {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  h2 { font-size: 18px; font-weight: 600; margin: 0; letter-spacing: -0.01em; }

  .group-count {
    font-size: 12px;
    color: var(--text-muted);
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
    padding: 2px 8px;
    border-radius: 10px;
  }

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

  .group-list {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .group-card {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    overflow: hidden;
    transition: border-color 0.15s;
  }
  .group-card:hover { border-color: var(--border-light); }

  .group-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 14px 18px;
    border-bottom: 1px solid var(--border);
    background: var(--bg-surface);
  }

  .group-meta {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  .group-tag {
    font-size: 15px;
    font-weight: 600;
    color: var(--text-primary);
    font-family: 'JetBrains Mono', monospace;
  }

  .strategy-badge {
    font-size: 11px;
    font-weight: 600;
    padding: 2px 8px;
    border-radius: 10px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .strategy-failover   { background: rgba(167,139,250,0.15); color: var(--accent-purple); }
  .strategy-loadbalance { background: rgba(79,109,245,0.15); color: var(--accent); }
  .strategy-quality    { background: rgba(52,211,153,0.15); color: var(--accent-green); }
  .strategy-urltest    { background: rgba(251,191,36,0.15); color: var(--accent-yellow); }
  .strategy-select     { background: rgba(248,113,113,0.15); color: var(--accent-red); }
  .strategy-default    { background: var(--bg-tertiary); color: var(--text-muted); }

  .selected-badge {
    font-size: 11px;
    color: var(--text-muted);
    padding: 2px 8px;
    background: var(--bg-tertiary);
    border-radius: 10px;
  }

  .group-actions {
    display: flex;
    gap: 6px;
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

  .member-list {
    padding: 6px 0;
  }

  .member-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 8px 18px;
    transition: background 0.1s;
  }
  .member-row:hover { background: var(--bg-hover); }
  .member-row.is-selected { background: var(--accent-subtle); }

  .member-info {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }

  .member-tag {
    font-size: 13px;
    color: var(--text-primary);
    font-family: 'JetBrains Mono', monospace;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .active-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--accent-green);
    flex-shrink: 0;
  }

  .member-right {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-shrink: 0;
  }

  .latency {
    font-size: 12px;
    font-family: 'JetBrains Mono', monospace;
    font-weight: 500;
    padding: 2px 6px;
    border-radius: 4px;
  }
  .lat-good { color: var(--accent-green); background: var(--accent-green-subtle); }
  .lat-ok   { color: var(--accent-yellow); background: var(--accent-yellow-subtle); }
  .lat-slow { color: var(--accent-red); background: var(--accent-red-subtle); }
  .lat-dead { color: var(--text-muted); background: var(--bg-tertiary); }

  .btn-select {
    background: var(--bg-tertiary);
    color: var(--text-secondary);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 4px 10px;
    cursor: pointer;
    font-size: 11px;
    font-weight: 500;
    font-family: inherit;
    transition: all 0.15s;
    white-space: nowrap;
  }
  .btn-select:hover:not(:disabled) { background: var(--accent-subtle); color: var(--accent); border-color: var(--accent); }
  .btn-select.active { background: var(--accent-subtle); color: var(--accent); border-color: var(--accent); cursor: default; }
  .btn-select:disabled { opacity: 0.5; cursor: default; }

  .no-members {
    padding: 12px 18px;
    font-size: 13px;
    color: var(--text-muted);
    font-style: italic;
  }
</style>
