<script lang="ts">
  import { Button, Input, Switch } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import {
    getConfig, putConfig, connect,
    addServer, setActiveServer,
    addSubscription, importConfig,
  } from '@/lib/api/endpoints'
  import { createWizard, type AddMethod } from './state.svelte'
  import DotProgress from './DotProgress.svelte'

  interface Props { onComplete: () => void }
  let { onComplete }: Props = $props()

  const w = createWizard()

  async function addServers(): Promise<boolean> {
    w.error = ''
    w.busy = true
    try {
      if (w.method === 'subscription') {
        if (!w.subscriptionUrl.trim()) {
          w.error = t('onboarding.errors.enterSubscriptionUrl')
          return false
        }
        const sub = await addSubscription('', w.subscriptionUrl.trim())
        if (sub.servers?.length) {
          w.addedServers = sub.servers
        } else if (sub.error) {
          w.error = sub.error
          return false
        }
      } else if (w.method === 'import') {
        if (!w.importData.trim()) {
          w.error = t('onboarding.errors.pasteConfig')
          return false
        }
        const res = await importConfig(w.importData.trim())
        if (res.error) { w.error = res.error; return false }
        w.addedServers = res.servers ?? []
        if (!w.addedServers.length) {
          w.error = t('onboarding.errors.noServersFound')
          return false
        }
        if (res.mesh_enabled) w.meshAvailable = true
      } else {
        if (!w.manualAddr.trim()) {
          w.error = t('onboarding.errors.enterServerAddress')
          return false
        }
        await addServer({
          addr: w.manualAddr.trim(),
          password: w.manualPassword.trim() || undefined,
          name: 'My Server',
        })
        w.addedServers = [{ addr: w.manualAddr.trim(), name: 'My Server' }]
      }
      return true
    } catch (err) {
      w.error = (err as Error).message || t('onboarding.errors.failedToAdd')
      return false
    } finally {
      w.busy = false
    }
  }

  async function finish(): Promise<void> {
    w.error = ''
    w.busy = true
    try {
      const cfg = await getConfig()
      if (w.addedServers[0] && !cfg.server?.addr) {
        await setActiveServer(w.addedServers[0])
      }

      let changed = false
      if (w.enableSystemProxy) {
        cfg.proxy.system_proxy = { enabled: true }
        changed = true
      }
      if (w.meshAvailable && w.enableMesh) {
        cfg.mesh = { enabled: true, p2p_enabled: true }
        changed = true
      }
      if (changed) await putConfig(cfg)

      await connect()
      onComplete()
    } catch (err) {
      w.error = (err as Error).message || t('onboarding.errors.failedToComplete')
    } finally {
      w.busy = false
    }
  }

  async function onNext(): Promise<void> {
    if (w.step === 2) {
      const ok = await addServers()
      if (!ok) return
      w.step = 3
      return
    }
    if (w.step === 4) {
      await finish()
      return
    }
    w.step = (w.step + 1) as typeof w.step
  }

  function onBack(): void {
    if (w.step > 1) w.step = (w.step - 1) as typeof w.step
  }

  function onKey(e: KeyboardEvent): void {
    if (e.key === 'Escape') onComplete()
  }

  const METHODS: { value: AddMethod; key: string }[] = [
    { value: 'subscription', key: 'onboarding.subscription' },
    { value: 'import',       key: 'onboarding.import' },
    { value: 'manual',       key: 'onboarding.manual' },
  ]

  const nextLabel = $derived.by(() => {
    if (w.step === 4) return w.busy ? t('onboarding.connecting') : t('onboarding.connectNow')
    if (w.step === 2) return w.busy ? t('onboarding.adding') : t('onboarding.next')
    return t('onboarding.next')
  })
</script>

<div class="overlay" role="dialog" aria-modal="true" aria-labelledby="ob-title" tabindex="-1" onkeydown={onKey}>
  <div class="wizard">
    <DotProgress total={4} current={w.step} />

    <div class="content">
      {#if w.step === 1}
        <h1 id="ob-title">{t('onboarding.welcome')}</h1>
        <p class="lede">{t('onboarding.subtitle')}</p>
        <ul class="features">
          <li>{t('onboarding.feature1')}</li>
          <li>{t('onboarding.feature2')}</li>
          <li>{t('onboarding.feature3')}</li>
        </ul>
      {:else if w.step === 2}
        <h1 id="ob-title">{t('onboarding.addServer')}</h1>
        <p class="lede">{t('onboarding.addServerDesc')}</p>
        <div class="tabs" role="tablist">
          {#each METHODS as m (m.value)}
            <button
              type="button"
              role="tab"
              class="tab"
              class:on={w.method === m.value}
              aria-selected={w.method === m.value}
              onclick={() => (w.method = m.value)}
            >
              {t(m.key)}
            </button>
          {/each}
        </div>

        <div class="form">
          {#if w.method === 'subscription'}
            <Input
              label={t('onboarding.subscriptionUrl')}
              bind:value={w.subscriptionUrl}
              placeholder="https://example.com/subscribe/..."
            />
            <p class="hint">{t('onboarding.subscriptionHint')}</p>
          {:else if w.method === 'import'}
            <label class="textarea-label">
              <span>{t('onboarding.configuration')}</span>
              <textarea
                bind:value={w.importData}
                placeholder="Paste ss://, vmess://, shuttle://, or JSON config..."
                rows="5"
              ></textarea>
            </label>
            <p class="hint">{t('onboarding.importHint')}</p>
          {:else}
            <Input
              label={t('onboarding.serverAddress')}
              bind:value={w.manualAddr}
              placeholder="server.example.com:443"
            />
            <Input
              label={t('onboarding.password')}
              type="password"
              bind:value={w.manualPassword}
              placeholder={t('onboarding.optional')}
            />
          {/if}
        </div>
      {:else if w.step === 3}
        <h1 id="ob-title">{t('onboarding.addServer')}</h1>
        <p class="lede">{t('onboarding.serversAdded', { count: w.addedServers.length })}</p>

        <ul class="server-list">
          {#each w.addedServers.slice(0, 3) as s (s.addr)}
            <li><span class="dot"></span><span class="name">{s.name || s.addr}</span></li>
          {/each}
          {#if w.addedServers.length > 3}
            <li class="more">+{w.addedServers.length - 3} {t('common.more')}</li>
          {/if}
        </ul>

        <div class="option">
          <div class="opt-text">
            <div class="opt-title">{t('onboarding.enableSystemProxy')}</div>
            <div class="opt-desc">{t('onboarding.systemProxyDesc')}</div>
          </div>
          <Switch bind:checked={w.enableSystemProxy} />
        </div>

        {#if w.meshAvailable}
          <div class="option">
            <div class="opt-text">
              <div class="opt-title">{t('onboarding.enableMesh')}</div>
              <div class="opt-desc">{t('onboarding.meshDesc')}</div>
            </div>
            <Switch bind:checked={w.enableMesh} />
          </div>
        {/if}
      {:else if w.step === 4}
        <h1 id="ob-title">{t('onboarding.ready')}</h1>
        <p class="lede">
          {w.addedServers.length > 0
            ? t('onboarding.serversAdded', { count: w.addedServers.length })
            : t('onboarding.subtitle')}
        </p>
      {/if}

      {#if w.error}<p class="err">{w.error}</p>{/if}
    </div>

    <footer>
      {#if w.step > 1}
        <Button variant="ghost" onclick={onBack} disabled={w.busy}>
          {t('onboarding.back')}
        </Button>
      {:else}
        <Button variant="ghost" onclick={onComplete}>
          {t('onboarding.skip')}
        </Button>
      {/if}
      <div class="spacer"></div>
      <Button variant="primary" loading={w.busy} onclick={onNext}>
        {nextLabel}
      </Button>
    </footer>
  </div>
</div>

<style>
  .overlay {
    position: fixed; inset: 0; z-index: 1000;
    display: flex; align-items: center; justify-content: center;
    background: color-mix(in oklab, var(--shuttle-bg-base) 70%, transparent);
    backdrop-filter: blur(10px);
    font-family: var(--shuttle-font-sans);
  }
  .wizard {
    width: 100%;
    max-width: 480px;
    background: var(--shuttle-bg-surface);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-lg);
    box-shadow: var(--shuttle-shadow-md);
    padding: var(--shuttle-space-6) var(--shuttle-space-6) var(--shuttle-space-4);
    display: flex;
    flex-direction: column;
    gap: var(--shuttle-space-5);
  }

  .content {
    display: flex;
    flex-direction: column;
    gap: var(--shuttle-space-4);
    min-height: 220px;
  }
  h1 {
    margin: 0;
    font-size: var(--shuttle-text-2xl);
    font-weight: var(--shuttle-weight-semibold);
    color: var(--shuttle-fg-primary);
    letter-spacing: -0.02em;
  }
  .lede {
    margin: 0;
    font-size: var(--shuttle-text-sm);
    color: var(--shuttle-fg-secondary);
  }

  .features {
    list-style: none;
    margin: var(--shuttle-space-2) 0 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--shuttle-space-2);
  }
  .features li {
    font-size: var(--shuttle-text-sm);
    color: var(--shuttle-fg-primary);
    padding-left: var(--shuttle-space-4);
    position: relative;
  }
  .features li::before {
    content: '';
    position: absolute;
    left: 0;
    top: 8px;
    width: 5px;
    height: 5px;
    border-radius: 50%;
    background: var(--shuttle-fg-primary);
  }

  .tabs {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 0;
    background: var(--shuttle-bg-subtle);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    padding: 3px;
  }
  .tab {
    appearance: none;
    background: transparent;
    border: none;
    color: var(--shuttle-fg-muted);
    font-size: var(--shuttle-text-sm);
    font-family: inherit;
    padding: 6px 0;
    border-radius: calc(var(--shuttle-radius-md) - 3px);
    cursor: pointer;
    transition: background 150ms, color 150ms;
  }
  .tab:hover { color: var(--shuttle-fg-primary); }
  .tab.on {
    background: var(--shuttle-bg-surface);
    color: var(--shuttle-fg-primary);
    box-shadow: var(--shuttle-shadow-sm);
  }

  .form {
    display: flex;
    flex-direction: column;
    gap: var(--shuttle-space-3);
  }
  .hint {
    margin: 0;
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-muted);
  }
  .textarea-label {
    display: flex;
    flex-direction: column;
    gap: var(--shuttle-space-1);
  }
  .textarea-label span {
    font-size: var(--shuttle-text-sm);
    color: var(--shuttle-fg-secondary);
    font-weight: var(--shuttle-weight-medium);
  }
  .textarea-label textarea {
    resize: vertical;
    min-height: 96px;
    padding: var(--shuttle-space-2) var(--shuttle-space-3);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    background: var(--shuttle-bg-surface);
    color: var(--shuttle-fg-primary);
    font-family: var(--shuttle-font-mono);
    font-size: var(--shuttle-text-xs);
    outline: none;
  }
  .textarea-label textarea:focus { border-color: var(--shuttle-border-strong); }

  .server-list {
    list-style: none;
    margin: 0;
    padding: var(--shuttle-space-2) 0;
    display: flex;
    flex-direction: column;
    gap: var(--shuttle-space-1);
    font-size: var(--shuttle-text-sm);
  }
  .server-list li {
    display: flex;
    align-items: center;
    gap: var(--shuttle-space-2);
    color: var(--shuttle-fg-primary);
  }
  .server-list .dot {
    width: 6px; height: 6px; border-radius: 50%;
    background: var(--shuttle-success);
  }
  .server-list .name {
    font-family: var(--shuttle-font-mono);
    font-size: var(--shuttle-text-xs);
  }
  .server-list .more {
    color: var(--shuttle-fg-muted);
    padding-left: 14px;
  }

  .option {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--shuttle-space-3);
    padding: var(--shuttle-space-3) 0;
    border-top: 1px solid var(--shuttle-border);
  }
  .option:first-of-type { border-top: none; }
  .opt-text { min-width: 0; }
  .opt-title {
    font-size: var(--shuttle-text-sm);
    font-weight: var(--shuttle-weight-medium);
    color: var(--shuttle-fg-primary);
  }
  .opt-desc {
    margin-top: 2px;
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-muted);
  }

  .err {
    margin: 0;
    padding: var(--shuttle-space-2) var(--shuttle-space-3);
    background: color-mix(in oklab, var(--shuttle-danger) 10%, transparent);
    border: 1px solid color-mix(in oklab, var(--shuttle-danger) 30%, transparent);
    border-radius: var(--shuttle-radius-sm);
    color: var(--shuttle-danger);
    font-size: var(--shuttle-text-xs);
  }

  footer {
    display: flex;
    align-items: center;
    padding-top: var(--shuttle-space-3);
    border-top: 1px solid var(--shuttle-border);
  }
  .spacer { flex: 1; }
</style>
