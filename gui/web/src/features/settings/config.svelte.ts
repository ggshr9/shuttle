import { getConfig, putConfig } from '@/lib/api/endpoints'
import type { Config } from '@/lib/api/types'
import { toasts } from '@/lib/toaster.svelte'
import { t } from '@/lib/i18n/index'

function clone<T>(v: T): T {
  return v === undefined ? v : JSON.parse(JSON.stringify(v)) as T
}

function equal(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) === JSON.stringify(b)
}

// Normalize the loaded config so sub-pages can bind without null guards.
function normalize(cfg: Config): Config {
  const c = cfg as Config & Record<string, unknown>
  c.proxy ??= {} as Config['proxy']
  const p = c.proxy
  p.socks5 ??= { enabled: false, listen: '127.0.0.1:1080' }
  p.http   ??= { enabled: false, listen: '127.0.0.1:8080' }
  p.tun    ??= { enabled: false, device_name: '', per_app_mode: '', app_list: [] }
  p.tun.app_list    ??= []
  p.tun.per_app_mode ??= ''
  p.system_proxy ??= { enabled: false }
  p.allow_lan ??= false

  c.mesh   ??= { enabled: false, p2p_enabled: false }
  c.log    ??= { level: 'info' }
  c.routing ??= { default: 'proxy', rules: [], dns: {}, geodata: {} }
  c.routing.dns ??= {}
  const dns = c.routing.dns
  dns.domestic ??= ''
  dns.remote   ??= { server: '', via: 'proxy' }
  dns.remote.server ??= ''
  dns.remote.via    ??= 'proxy'
  dns.cache    ??= true
  dns.prefetch ??= false
  c.routing.geodata ??= {}
  c.routing.geodata.enabled     ??= true
  c.routing.geodata.auto_update ??= true
  c.qos    ??= { enabled: false, rules: [] }
  c.qos.rules ??= []
  return cfg
}

class SettingsStore {
  pristine = $state<Config | null>(null)
  draft    = $state<Config | null>(null)
  loading  = $state(true)
  saving   = $state(false)
  error    = $state<string | null>(null)

  isDirty = $derived(
    this.pristine !== null && this.draft !== null && !equal(this.pristine, this.draft),
  )

  // In-flight promise dedups concurrent callers; cleared on settle so a
  // transient failure can be retried by revisiting the page.
  #inflight: Promise<void> | null = null

  ensureLoaded(): Promise<void> {
    if (this.pristine !== null) return Promise.resolve()
    if (this.#inflight) return this.#inflight
    this.#inflight = this.load().finally(() => { this.#inflight = null })
    return this.#inflight
  }

  async load(): Promise<void> {
    this.loading = true
    this.error = null
    try {
      const cfg = normalize(await getConfig())
      this.pristine = clone(cfg)
      this.draft    = clone(cfg)
    } catch (e) {
      this.error = (e as Error).message
    } finally {
      this.loading = false
    }
  }

  async save(): Promise<void> {
    if (!this.draft) return
    this.saving = true
    try {
      await putConfig(this.draft)
      this.pristine = clone(this.draft)
      toasts.success(t('settings.saved'))
    } catch (e) {
      toasts.error((e as Error).message)
      throw e
    } finally {
      this.saving = false
    }
  }

  discard(): void {
    if (!this.pristine) return
    this.draft = clone(this.pristine)
  }
}

export const settings = new SettingsStore()
