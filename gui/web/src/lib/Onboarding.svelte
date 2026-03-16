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
        <div class="icon">🚀</div>
        <h2 id="onboarding-title">{t('onboarding.welcome')}</h2>
        <p class="subtitle">{t('onboarding.subtitle')}</p>

        <div class="features">
          <div class="feature">
            <span class="emoji">⚡</span>
            <span>{t('onboarding.feature1')}</span>
          </div>
          <div class="feature">
            <span class="emoji">🔒</span>
            <span>{t('onboarding.feature2')}</span>
          </div>
          <div class="feature">
            <span class="emoji">🌍</span>
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
        <div class="icon success">✓</div>
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
    background: rgba(0, 0, 0, 0.85);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
  }

  .wizard {
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 16px;
    width: 100%;
    max-width: 440px;
    padding: 32px;
    box-shadow: 0 16px 48px rgba(0, 0, 0, 0.4);
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
    background: var(--border);
    color: var(--text-secondary);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 14px;
    font-weight: 600;
    transition: all 0.3s;
  }

  .progress .step.active {
    background: var(--btn-bg);
    color: white;
  }

  .progress .step.done {
    background: var(--btn-bg);
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
    background: var(--btn-bg);
  }

  .content {
    text-align: center;
  }

  .icon {
    font-size: 48px;
    margin-bottom: 16px;
  }

  .icon.success {
    width: 64px;
    height: 64px;
    background: var(--btn-bg);
    border-radius: 50%;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-size: 32px;
    color: white;
  }

  h2 {
    font-size: 24px;
    margin: 0 0 8px;
    color: var(--text-primary);
  }

  .subtitle {
    color: var(--text-secondary);
    margin: 0 0 24px;
    font-size: 14px;
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
    border-radius: 8px;
    margin-bottom: 8px;
    font-size: 14px;
  }

  .feature .emoji {
    font-size: 20px;
  }

  .method-tabs {
    display: flex;
    gap: 4px;
    background: var(--bg-surface);
    border-radius: 8px;
    padding: 4px;
    margin-bottom: 20px;
  }

  .method-tabs button {
    flex: 1;
    padding: 10px;
    border: none;
    background: transparent;
    color: var(--text-secondary);
    border-radius: 6px;
    cursor: pointer;
    font-size: 13px;
    transition: all 0.2s;
  }

  .method-tabs button:hover {
    color: var(--text-primary);
  }

  .method-tabs button.active {
    background: var(--btn-bg);
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
  }

  .form input, .form textarea {
    width: 100%;
    padding: 10px 12px;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text-primary);
    font-size: 14px;
    box-sizing: border-box;
  }

  .form input:focus, .form textarea:focus {
    outline: none;
    border-color: var(--accent);
  }

  .form textarea {
    resize: vertical;
    font-family: monospace;
  }

  .hint {
    font-size: 12px;
    color: var(--text-secondary);
    margin: 4px 0 0;
    text-align: left;
  }

  .error {
    color: var(--accent-red);
    font-size: 13px;
    margin: 12px 0;
    padding: 8px 12px;
    background: rgba(248, 81, 73, 0.1);
    border-radius: 6px;
  }

  .buttons {
    display: flex;
    gap: 12px;
    margin-bottom: 12px;
  }

  button.primary {
    flex: 1;
    padding: 12px 24px;
    background: var(--btn-bg);
    color: white;
    border: none;
    border-radius: 8px;
    font-size: 15px;
    font-weight: 500;
    cursor: pointer;
    transition: background 0.2s;
  }

  button.primary:hover:not(:disabled) {
    background: var(--btn-bg-hover);
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
    background: var(--border);
    color: var(--text-primary);
    border: none;
    border-radius: 8px;
    font-size: 15px;
    cursor: pointer;
    transition: background 0.2s;
  }

  button.secondary:hover {
    background: #3d444d;
  }

  button.text {
    background: none;
    border: none;
    color: var(--text-secondary);
    font-size: 13px;
    cursor: pointer;
    padding: 8px;
  }

  button.text:hover {
    color: var(--text-primary);
  }

  .server-preview {
    background: var(--bg-surface);
    border-radius: 8px;
    padding: 12px;
    margin-bottom: 20px;
    text-align: left;
  }

  .server-item {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 0;
    font-size: 13px;
    color: var(--text-primary);
  }

  .server-item .dot {
    width: 8px;
    height: 8px;
    background: var(--btn-bg);
    border-radius: 50%;
  }

  .server-item.more {
    color: var(--text-secondary);
    padding-left: 16px;
  }

  .checkbox-option {
    display: flex;
    align-items: flex-start;
    gap: 12px;
    background: var(--bg-surface);
    border-radius: 8px;
    padding: 16px;
    margin-bottom: 24px;
    cursor: pointer;
    text-align: left;
  }

  .checkbox-option input {
    margin-top: 2px;
    width: 18px;
    height: 18px;
    accent-color: var(--btn-bg);
  }

  .checkbox-option .option-title {
    display: block;
    font-size: 14px;
    color: var(--text-primary);
    margin-bottom: 2px;
  }

  .checkbox-option .option-desc {
    display: block;
    font-size: 12px;
    color: var(--text-secondary);
  }
</style>
