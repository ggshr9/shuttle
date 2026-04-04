<script lang="ts">
  import { api } from './api'
  import { t } from './i18n/index'

  let { onComplete } = $props()

  let step = $state(1)
  let loading = $state(false)
  let error = $state('')

  // Step 2: Add server
  let addMethod = $state('subscription') // 'subscription', 'import', 'manual'
  let subscriptionUrl = $state('')
  let importData = $state('')
  let manualAddr = $state('')
  let manualPassword = $state('')

  // Step 3: Options
  let enableSystemProxy = $state(true)
  let enableMesh = $state(true)
  let meshAvailable = $state(false)

  // Results
  let addedServers = $state([])

  async function handleAddServer() {
    loading = true
    error = ''

    try {
      if (addMethod === 'subscription') {
        if (!subscriptionUrl.trim()) {
          error = t('onboarding.errors.enterSubscriptionUrl')
          return
        }
        // Add subscription
        const sub = await api.addSubscription('', subscriptionUrl.trim())
        if (sub.servers && sub.servers.length > 0) {
          addedServers = sub.servers
        } else if (sub.error) {
          error = sub.error
          return
        }
      } else if (addMethod === 'import') {
        if (!importData.trim()) {
          error = t('onboarding.errors.pasteConfig')
          return
        }
        const result = await api.importConfig(importData.trim())
        if (result.error) {
          error = result.error
          return
        }
        addedServers = result.servers || []
        if (addedServers.length === 0) {
          error = t('onboarding.errors.noServersFound')
          return
        }
        if (result.mesh_enabled) {
          meshAvailable = true
        }
      } else if (addMethod === 'manual') {
        if (!manualAddr.trim()) {
          error = t('onboarding.errors.enterServerAddress')
          return
        }
        await api.addServer({
          addr: manualAddr.trim(),
          password: manualPassword.trim(),
          name: 'My Server'
        })
        addedServers = [{ addr: manualAddr.trim(), name: 'My Server' }]
      }

      // Success - go to next step
      step = 3
    } catch (err) {
      error = err.message || t('onboarding.errors.failedToAdd')
    } finally {
      loading = false
    }
  }

  async function handleComplete() {
    loading = true
    error = ''

    try {
      // Set the first server as active if we have servers
      if (addedServers.length > 0) {
        const cfg = await api.getConfig()
        if (!cfg.server?.addr && addedServers[0]) {
          await api.setActiveServer(addedServers[0])
        }
      }

      // Apply options
      const cfg = await api.getConfig()
      let configChanged = false

      if (enableSystemProxy) {
        if (!cfg.proxy.system_proxy) {
          cfg.proxy.system_proxy = { enabled: true }
        } else {
          cfg.proxy.system_proxy.enabled = true
        }
        configChanged = true
      }

      if (meshAvailable && enableMesh) {
        if (!cfg.mesh) {
          cfg.mesh = { enabled: true, p2p_enabled: true }
        } else {
          cfg.mesh.enabled = true
          cfg.mesh.p2p_enabled = true
        }
        configChanged = true
      }

      if (configChanged) {
        await api.putConfig(cfg)
      }

      // Connect
      await api.connect()

      onComplete?.()
    } catch (err) {
      error = err.message || t('onboarding.errors.failedToComplete')
    } finally {
      loading = false
    }
  }

  function skip() {
    onComplete?.()
  }
</script>

<div
  class="overlay"
  role="dialog"
  aria-modal="true"
  aria-labelledby="onboarding-title"
  onkeydown={(e) => e.key === 'Escape' && skip()}
>
  <div class="wizard">
    <!-- Progress indicator -->
    <div class="progress">
      <div class="step" class:active={step >= 1} class:done={step > 1}>1</div>
      <div class="line" class:done={step > 1}></div>
      <div class="step" class:active={step >= 2} class:done={step > 2}>2</div>
      <div class="line" class:done={step > 2}></div>
      <div class="step" class:active={step >= 3}>3</div>
    </div>

    <!-- Step 1: Welcome -->
    {#if step === 1}
      <div class="content">
        <div class="icon-wrap">
          <svg width="48" height="48" viewBox="0 0 48 48" fill="none">
            <rect width="48" height="48" rx="14" fill="var(--accent)"/>
            <path d="M14 24l6-9 6 9-6 9-6-9zm6-3l6 9h-12l6-9z" fill="white" opacity="0.9"/>
            <path d="M24 18l6 6-6 6" stroke="white" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
        </div>
        <h2 id="onboarding-title">{t('onboarding.welcome')}</h2>
        <p class="subtitle">{t('onboarding.subtitle')}</p>

        <div class="features">
          <div class="feature">
            <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="var(--accent)" stroke-width="1.5"><path d="M13 2L7 18M3 7l4 3-4 3m10 0h4"/></svg>
            <span>{t('onboarding.feature1')}</span>
          </div>
          <div class="feature">
            <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="var(--accent-green)" stroke-width="1.5"><rect x="3" y="7" width="14" height="9" rx="2"/><path d="M7 7V5a3 3 0 016 0v2"/></svg>
            <span>{t('onboarding.feature2')}</span>
          </div>
          <div class="feature">
            <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="var(--accent-purple)" stroke-width="1.5"><circle cx="10" cy="10" r="7"/><path d="M3 10h14M10 3a11.95 11.95 0 013 7 11.95 11.95 0 01-3 7 11.95 11.95 0 01-3-7 11.95 11.95 0 013-7z"/></svg>
            <span>{t('onboarding.feature3')}</span>
          </div>
        </div>

        <button class="primary" onclick={() => step = 2}>
          {t('onboarding.getStarted')}
        </button>
        <button class="text" onclick={skip}>
          {t('onboarding.skip')}
        </button>
      </div>
    {/if}

    <!-- Step 2: Add Server -->
    {#if step === 2}
      <div class="content">
        <h2>{t('onboarding.addServer')}</h2>
        <p class="subtitle">{t('onboarding.addServerDesc')}</p>

        <div class="method-tabs">
          <button
            class:active={addMethod === 'subscription'}
            onclick={() => addMethod = 'subscription'}
          >
            {t('onboarding.subscription')}
          </button>
          <button
            class:active={addMethod === 'import'}
            onclick={() => addMethod = 'import'}
          >
            {t('onboarding.import')}
          </button>
          <button
            class:active={addMethod === 'manual'}
            onclick={() => addMethod = 'manual'}
          >
            {t('onboarding.manual')}
          </button>
        </div>

        <div class="form">
          {#if addMethod === 'subscription'}
            <label>
              <span>{t('onboarding.subscriptionUrl')}</span>
              <input
                type="url"
                bind:value={subscriptionUrl}
                placeholder="https://example.com/subscribe/..."
              />
            </label>
            <p class="hint">{t('onboarding.subscriptionHint')}</p>
          {:else if addMethod === 'import'}
            <label>
              <span>{t('onboarding.configuration')}</span>
              <textarea
                bind:value={importData}
                placeholder="Paste ss://, vmess://, shuttle://, or JSON config..."
                rows="4"
              ></textarea>
            </label>
            <p class="hint">{t('onboarding.importHint')}</p>
          {:else if addMethod === 'manual'}
            <label>
              <span>{t('onboarding.serverAddress')}</span>
              <input
                type="text"
                bind:value={manualAddr}
                placeholder="server.example.com:443"
              />
            </label>
            <label>
              <span>{t('onboarding.password')}</span>
              <input
                type="password"
                bind:value={manualPassword}
                placeholder={t('onboarding.optional')}
              />
            </label>
          {/if}
        </div>

        {#if error}
          <p class="error">{error}</p>
        {/if}

        <div class="buttons">
          <button class="secondary" onclick={() => step = 1}>{t('onboarding.back')}</button>
          <button class="primary" onclick={handleAddServer} disabled={loading}>
            {loading ? t('onboarding.adding') : t('onboarding.addServerBtn')}
          </button>
        </div>
        <button class="text" onclick={skip}>{t('onboarding.skipForNow')}</button>
      </div>
    {/if}

    <!-- Step 3: Complete -->
    {#if step === 3}
      <div class="content">
        <div class="icon-wrap success">
          <svg width="32" height="32" viewBox="0 0 32 32" fill="none" stroke="white" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M8 16l6 6 10-12"/></svg>
        </div>
        <h2>{t('onboarding.ready')}</h2>
        <p class="subtitle">
          {t('onboarding.serversAdded', { count: addedServers.length })}
        </p>

        <div class="server-preview">
          {#each addedServers.slice(0, 3) as server}
            <div class="server-item">
              <span class="dot"></span>
              <span>{server.name || server.addr}</span>
            </div>
          {/each}
          {#if addedServers.length > 3}
            <div class="server-item more">
              +{addedServers.length - 3} {t('common.more')}
            </div>
          {/if}
        </div>

        <label class="checkbox-option">
          <input type="checkbox" bind:checked={enableSystemProxy} />
          <div>
            <span class="option-title">{t('onboarding.enableSystemProxy')}</span>
            <span class="option-desc">{t('onboarding.systemProxyDesc')}</span>
          </div>
        </label>

        {#if meshAvailable}
          <label class="checkbox-option">
            <input type="checkbox" bind:checked={enableMesh} />
            <div>
              <span class="option-title">{t('onboarding.enableMesh')}</span>
              <span class="option-desc">{t('onboarding.meshDesc')}</span>
            </div>
          </label>
        {/if}

        {#if error}
          <p class="error">{error}</p>
        {/if}

        <button class="primary large" onclick={handleComplete} disabled={loading}>
          {loading ? t('onboarding.connecting') : t('onboarding.connectNow')}
        </button>
        <button class="text" onclick={skip}>{t('onboarding.configureLater')}</button>
      </div>
    {/if}
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background: rgba(0, 0, 0, 0.8);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
    backdrop-filter: blur(8px);
  }

  .wizard {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: var(--radius-xl);
    width: 100%;
    max-width: 440px;
    padding: 32px;
    box-shadow: var(--shadow-lg);
  }

  .progress {
    display: flex;
    align-items: center;
    justify-content: center;
    margin-bottom: 32px;
  }

  .progress .step {
    width: 32px;
    height: 32px;
    border-radius: 50%;
    background: var(--bg-tertiary);
    color: var(--text-muted);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 13px;
    font-weight: 600;
    transition: all 0.3s;
  }

  .progress .step.active {
    background: var(--accent);
    color: white;
  }

  .progress .step.done {
    background: var(--accent);
    color: white;
  }

  .progress .line {
    width: 48px;
    height: 2px;
    background: var(--border);
    margin: 0 8px;
    transition: background 0.3s;
  }

  .progress .line.done {
    background: var(--accent);
  }

  .content {
    text-align: center;
  }

  .icon-wrap {
    margin-bottom: 16px;
    display: inline-block;
  }

  .icon-wrap.success {
    width: 64px;
    height: 64px;
    background: var(--accent-green);
    border-radius: 50%;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }

  h2 {
    font-size: 22px;
    font-weight: 700;
    margin: 0 0 8px;
    color: var(--text-primary);
    letter-spacing: -0.02em;
  }

  .subtitle {
    color: var(--text-secondary);
    margin: 0 0 24px;
    font-size: 14px;
    line-height: 1.5;
  }

  .features {
    text-align: left;
    margin-bottom: 32px;
  }

  .feature {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px 16px;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    margin-bottom: 8px;
    font-size: 14px;
    color: var(--text-primary);
  }

  .method-tabs {
    display: flex;
    gap: 4px;
    background: var(--bg-surface);
    border-radius: var(--radius-md);
    padding: 4px;
    margin-bottom: 20px;
  }

  .method-tabs button {
    flex: 1;
    padding: 10px;
    border: none;
    background: transparent;
    color: var(--text-secondary);
    border-radius: var(--radius-sm);
    cursor: pointer;
    font-size: 13px;
    font-weight: 500;
    font-family: inherit;
    transition: all 0.2s;
  }

  .method-tabs button:hover {
    color: var(--text-primary);
  }

  .method-tabs button.active {
    background: var(--accent);
    color: white;
  }

  .form {
    text-align: left;
    margin-bottom: 16px;
  }

  .form label {
    display: block;
    margin-bottom: 12px;
  }

  .form label span {
    display: block;
    font-size: 13px;
    color: var(--text-secondary);
    margin-bottom: 6px;
    font-weight: 500;
  }

  .form input, .form textarea {
    width: 100%;
    padding: 10px 12px;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    color: var(--text-primary);
    font-size: 14px;
    box-sizing: border-box;
    font-family: inherit;
    transition: border-color 0.15s;
  }

  .form input:focus, .form textarea:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-subtle);
  }

  .form textarea {
    resize: vertical;
    font-family: 'JetBrains Mono', monospace;
  }

  .hint {
    font-size: 12px;
    color: var(--text-muted);
    margin: 4px 0 0;
    text-align: left;
  }

  .error {
    color: var(--accent-red);
    font-size: 13px;
    margin: 12px 0;
    padding: 10px 14px;
    background: var(--accent-red-subtle);
    border-radius: var(--radius-sm);
  }

  .buttons {
    display: flex;
    gap: 12px;
    margin-bottom: 12px;
  }

  button.primary {
    flex: 1;
    padding: 12px 24px;
    background: var(--accent);
    color: white;
    border: none;
    border-radius: var(--radius-md);
    font-size: 15px;
    font-weight: 600;
    font-family: inherit;
    cursor: pointer;
    transition: background 0.2s;
  }

  button.primary:hover:not(:disabled) {
    background: var(--accent-hover);
  }

  button.primary:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  button.primary.large {
    padding: 14px 32px;
    font-size: 16px;
  }

  button.secondary {
    padding: 12px 24px;
    background: var(--bg-tertiary);
    color: var(--text-primary);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    font-size: 15px;
    font-family: inherit;
    cursor: pointer;
    transition: background 0.2s;
  }

  button.secondary:hover {
    background: var(--bg-hover);
  }

  button.text {
    background: none;
    border: none;
    color: var(--text-muted);
    font-size: 13px;
    font-family: inherit;
    cursor: pointer;
    padding: 8px;
  }

  button.text:hover {
    color: var(--text-primary);
  }

  .server-preview {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    padding: 12px 16px;
    margin-bottom: 20px;
    text-align: left;
  }

  .server-item {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 0;
    font-size: 13px;
    color: var(--text-primary);
  }

  .server-item .dot {
    width: 8px;
    height: 8px;
    background: var(--accent-green);
    border-radius: 50%;
  }

  .server-item.more {
    color: var(--text-muted);
    padding-left: 18px;
  }

  .checkbox-option {
    display: flex;
    align-items: flex-start;
    gap: 12px;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    padding: 16px;
    margin-bottom: 12px;
    cursor: pointer;
    text-align: left;
    transition: border-color 0.15s;
  }

  .checkbox-option:hover {
    border-color: var(--accent);
  }

  .checkbox-option input {
    margin-top: 2px;
    width: 18px;
    height: 18px;
    accent-color: var(--accent);
  }

  .checkbox-option .option-title {
    display: block;
    font-size: 14px;
    font-weight: 500;
    color: var(--text-primary);
    margin-bottom: 2px;
  }

  .checkbox-option .option-desc {
    display: block;
    font-size: 12px;
    color: var(--text-secondary);
  }
</style>
