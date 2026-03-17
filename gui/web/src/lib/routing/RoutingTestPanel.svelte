<script lang="ts">
  import { t } from '../i18n/index'
  import { api } from '../api'

  let testUrl = $state('')
  let testing = $state(false)
  let testResult = $state(null)
  let testError = $state('')

  function actionColor(action) {
    switch (action) {
      case 'proxy': return 'var(--accent)'
      case 'direct': return 'var(--accent-green)'
      case 'reject': return 'var(--accent-red)'
      default: return 'var(--text-secondary)'
    }
  }

  async function runTest() {
    if (!testUrl.trim()) return
    testing = true
    testResult = null
    testError = ''
    try {
      testResult = await api.testRouting(testUrl.trim())
    } catch (e) {
      testError = e.message
    } finally {
      testing = false
    }
  }
</script>

<div class="test-section">
  <span class="test-label">{t('routing.testUrl')}</span>
  <div class="test-row">
    <input
      class="test-input"
      bind:value={testUrl}
      placeholder={t('routing.testPlaceholder')}
      onkeydown={(e) => e.key === 'Enter' && runTest()}
    />
    <button class="test-btn" onclick={runTest} disabled={testing || !testUrl.trim()}>
      {testing ? t('routing.testing') : t('routing.test')}
    </button>
  </div>
  {#if testResult}
    <div class="test-result">
      <span class="test-result-action" style="color: {actionColor(testResult.action)}">
        {testResult.action.toUpperCase()}
      </span>
      <span class="test-result-detail">
        {t('routing.matchedBy')}: <strong>{testResult.matched_by}</strong>
        {#if testResult.rule}
          &mdash; {testResult.rule}
        {/if}
      </span>
    </div>
  {/if}
  {#if testError}
    <p class="test-error">{testError}</p>
  {/if}
</div>

<style>
  .test-section {
    margin-bottom: 20px;
    padding: 12px;
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 8px;
  }

  .test-label {
    font-size: 13px;
    color: var(--text-secondary);
    display: block;
    margin-bottom: 8px;
  }

  .test-row {
    display: flex;
    gap: 8px;
  }

  .test-input {
    flex: 1;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px 12px;
    color: var(--text-primary);
    font-size: 13px;
  }

  .test-input:focus { outline: none; border-color: var(--accent); }

  .test-btn {
    background: var(--bg-tertiary);
    color: var(--accent);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px 16px;
    cursor: pointer;
    font-size: 13px;
    white-space: nowrap;
  }

  .test-btn:hover { background: #30363d; }
  .test-btn:disabled { opacity: 0.5; cursor: default; }

  .test-result {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-top: 10px;
    padding: 8px 12px;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    font-size: 13px;
  }

  .test-result-action {
    font-weight: 600;
    font-size: 12px;
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }

  .test-result-detail {
    color: var(--text-secondary);
  }

  .test-result-detail strong {
    color: var(--text-primary);
  }

  .test-error {
    font-size: 12px;
    color: var(--accent-red);
    margin: 8px 0 0;
  }
</style>
