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

  // Results
  let addedServers = $state([])

  async function handleAddServer() {
    loading = true
    error = ''

    try {
      if (addMethod === 'subscription') {
        if (!subscriptionUrl.trim()) {
          error = 'Please enter a subscription URL'
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
          error = 'Please paste configuration data'
          return
        }
        const result = await api.importConfig(importData.trim())
        if (result.error) {
          error = result.error
          return
        }
        addedServers = result.servers || []
        if (addedServers.length === 0) {
          error = 'No servers found in the configuration'
          return
        }
      } else if (addMethod === 'manual') {
        if (!manualAddr.trim()) {
          error = 'Please enter server address'
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
      error = err.message || 'Failed to add server'
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

      // Enable system proxy if selected
      if (enableSystemProxy) {
        const cfg = await api.getConfig()
        if (!cfg.proxy.system_proxy) {
          cfg.proxy.system_proxy = { enabled: true }
        } else {
          cfg.proxy.system_proxy.enabled = true
        }
        await api.putConfig(cfg)
      }

      // Connect
      await api.connect()

      onComplete?.()
    } catch (err) {
      error = err.message || 'Failed to complete setup'
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
        <h2 id="onboarding-title">Welcome to Shuttle</h2>
        <p class="subtitle">Fast, secure proxy for unrestricted internet access</p>

        <div class="features">
          <div class="feature">
            <span class="emoji">⚡</span>
            <span>Multiple protocols (H3, Reality, CDN)</span>
          </div>
          <div class="feature">
            <span class="emoji">🔒</span>
            <span>Advanced encryption & obfuscation</span>
          </div>
          <div class="feature">
            <span class="emoji">🌍</span>
            <span>Smart routing with GeoIP rules</span>
          </div>
        </div>

        <button class="primary" onclick={() => step = 2}>
          Get Started
        </button>
        <button class="text" onclick={skip}>
          Skip setup
        </button>
      </div>
    {/if}

    <!-- Step 2: Add Server -->
    {#if step === 2}
      <div class="content">
        <h2>Add a Server</h2>
        <p class="subtitle">Choose how to add your proxy server</p>

        <div class="method-tabs">
          <button
            class:active={addMethod === 'subscription'}
            onclick={() => addMethod = 'subscription'}
          >
            Subscription
          </button>
          <button
            class:active={addMethod === 'import'}
            onclick={() => addMethod = 'import'}
          >
            Import
          </button>
          <button
            class:active={addMethod === 'manual'}
            onclick={() => addMethod = 'manual'}
          >
            Manual
          </button>
        </div>

        <div class="form">
          {#if addMethod === 'subscription'}
            <label>
              <span>Subscription URL</span>
              <input
                type="url"
                bind:value={subscriptionUrl}
                placeholder="https://example.com/subscribe/..."
              />
            </label>
            <p class="hint">Paste the subscription link from your provider</p>
          {:else if addMethod === 'import'}
            <label>
              <span>Configuration</span>
              <textarea
                bind:value={importData}
                placeholder="Paste ss://, vmess://, shuttle://, or JSON config..."
                rows="4"
              ></textarea>
            </label>
            <p class="hint">Supports Base64, JSON, and URI formats</p>
          {:else if addMethod === 'manual'}
            <label>
              <span>Server Address</span>
              <input
                type="text"
                bind:value={manualAddr}
                placeholder="server.example.com:443"
              />
            </label>
            <label>
              <span>Password</span>
              <input
                type="password"
                bind:value={manualPassword}
                placeholder="Optional"
              />
            </label>
          {/if}
        </div>

        {#if error}
          <p class="error">{error}</p>
        {/if}

        <div class="buttons">
          <button class="secondary" onclick={() => step = 1}>Back</button>
          <button class="primary" onclick={handleAddServer} disabled={loading}>
            {loading ? 'Adding...' : 'Add Server'}
          </button>
        </div>
        <button class="text" onclick={skip}>Skip for now</button>
      </div>
    {/if}

    <!-- Step 3: Complete -->
    {#if step === 3}
      <div class="content">
        <div class="icon success">✓</div>
        <h2>Ready to Connect!</h2>
        <p class="subtitle">
          {addedServers.length} server{addedServers.length !== 1 ? 's' : ''} added successfully
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
              +{addedServers.length - 3} more
            </div>
          {/if}
        </div>

        <label class="checkbox-option">
          <input type="checkbox" bind:checked={enableSystemProxy} />
          <div>
            <span class="option-title">Enable System Proxy</span>
            <span class="option-desc">Automatically configure system proxy on connect</span>
          </div>
        </label>

        {#if error}
          <p class="error">{error}</p>
        {/if}

        <button class="primary large" onclick={handleComplete} disabled={loading}>
          {loading ? 'Connecting...' : 'Connect Now'}
        </button>
        <button class="text" onclick={skip}>Configure later</button>
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
    background: #161b22;
    border: 1px solid #2d333b;
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
    background: #2d333b;
    color: #8b949e;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 14px;
    font-weight: 600;
    transition: all 0.3s;
  }

  .progress .step.active {
    background: #238636;
    color: white;
  }

  .progress .step.done {
    background: #238636;
    color: white;
  }

  .progress .line {
    width: 48px;
    height: 2px;
    background: #2d333b;
    margin: 0 8px;
    transition: background 0.3s;
  }

  .progress .line.done {
    background: #238636;
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
    background: #238636;
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
    color: #e1e4e8;
  }

  .subtitle {
    color: #8b949e;
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
    background: #0d1117;
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
    background: #0d1117;
    border-radius: 8px;
    padding: 4px;
    margin-bottom: 20px;
  }

  .method-tabs button {
    flex: 1;
    padding: 10px;
    border: none;
    background: transparent;
    color: #8b949e;
    border-radius: 6px;
    cursor: pointer;
    font-size: 13px;
    transition: all 0.2s;
  }

  .method-tabs button:hover {
    color: #e1e4e8;
  }

  .method-tabs button.active {
    background: #238636;
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
    color: #8b949e;
    margin-bottom: 6px;
  }

  .form input, .form textarea {
    width: 100%;
    padding: 10px 12px;
    background: #0d1117;
    border: 1px solid #2d333b;
    border-radius: 8px;
    color: #e1e4e8;
    font-size: 14px;
    box-sizing: border-box;
  }

  .form input:focus, .form textarea:focus {
    outline: none;
    border-color: #58a6ff;
  }

  .form textarea {
    resize: vertical;
    font-family: monospace;
  }

  .hint {
    font-size: 12px;
    color: #8b949e;
    margin: 4px 0 0;
    text-align: left;
  }

  .error {
    color: #f85149;
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
    background: #238636;
    color: white;
    border: none;
    border-radius: 8px;
    font-size: 15px;
    font-weight: 500;
    cursor: pointer;
    transition: background 0.2s;
  }

  button.primary:hover:not(:disabled) {
    background: #2ea043;
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
    background: #2d333b;
    color: #e1e4e8;
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
    color: #8b949e;
    font-size: 13px;
    cursor: pointer;
    padding: 8px;
  }

  button.text:hover {
    color: #e1e4e8;
  }

  .server-preview {
    background: #0d1117;
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
    color: #e1e4e8;
  }

  .server-item .dot {
    width: 8px;
    height: 8px;
    background: #238636;
    border-radius: 50%;
  }

  .server-item.more {
    color: #8b949e;
    padding-left: 16px;
  }

  .checkbox-option {
    display: flex;
    align-items: flex-start;
    gap: 12px;
    background: #0d1117;
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
    accent-color: #238636;
  }

  .checkbox-option .option-title {
    display: block;
    font-size: 14px;
    color: #e1e4e8;
    margin-bottom: 2px;
  }

  .checkbox-option .option-desc {
    display: block;
    font-size: 12px;
    color: #8b949e;
  }
</style>
