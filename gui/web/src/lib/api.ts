// API client for Shuttle GUI

const BASE = ''

interface RequestOptions extends RequestInit {
  headers: Record<string, string>
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const opts: RequestOptions = {
    method,
    headers: { 'Content-Type': 'application/json' },
  }
  if (body) opts.body = JSON.stringify(body)
  const res = await fetch(BASE + path, opts)
  const data = await res.json()
  if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`)
  return data as T
}

// Types
export interface Server {
  addr: string
  name?: string
  password?: string
}

export interface ServersResponse {
  active: Server
  servers: Server[]
}

export interface Config {
  server?: Server
  servers?: Server[]
  proxy: {
    system_proxy?: {
      enabled: boolean
    }
  }
}

export interface ImportResult {
  added: number
  total: number
  servers?: Server[]
  errors?: string[]
  error?: string
}

export interface Subscription {
  id: string
  name: string
  url: string
  servers?: Server[]
  updated_at?: string
  error?: string
}

export interface SpeedtestResult {
  server_addr: string
  available: boolean
  latency: number
}

export interface AutoSelectResult {
  server: Server
  latency: number
}

export interface RoutingRules {
  rules: RoutingRule[]
  default: string
}

export interface RoutingRule {
  geosite?: string
  geoip?: string
  domain?: string
  process?: string
  action: string
}

export interface RoutingTemplate {
  id: string
  name: string
  description: string
}

export interface Process {
  name: string
  conns: number
}

export interface StatsHistory {
  [date: string]: {
    upload: number
    download: number
  }
}

export interface UpdateInfo {
  available: boolean
  version?: string
  release_notes?: string
  download_url?: string
}

export interface VersionInfo {
  version: string
  commit?: string
  build_date?: string
}

export interface LanInfo {
  ip: string
  gateway: string
  interfaces: string[]
}

export interface Status {
  connected: boolean
  server?: Server
  uptime?: number
  bytes_sent?: number
  bytes_recv?: number
}

export const api = {
  status: () => request<Status>('GET', '/api/status'),
  connect: () => request<void>('POST', '/api/connect'),
  disconnect: () => request<void>('POST', '/api/disconnect'),
  getConfig: () => request<Config>('GET', '/api/config'),
  putConfig: (cfg: Config) => request<void>('PUT', '/api/config', cfg),
  getServers: () => request<ServersResponse>('GET', '/api/config/servers'),
  setActiveServer: (srv: Server) => request<void>('PUT', '/api/config/servers', srv),
  putServers: (srv: Server) => request<void>('PUT', '/api/config/servers', srv),
  addServer: (srv: Server) => request<void>('POST', '/api/config/servers', srv),
  deleteServer: (addr: string) => request<void>('DELETE', '/api/config/servers', { addr }),
  autoSelectServer: () => request<AutoSelectResult>('POST', '/api/config/servers/auto-select'),
  importConfig: (data: string) => request<ImportResult>('POST', '/api/config/import', { data }),
  exportConfig: (format = 'json') => `${BASE}/api/config/export?format=${format}`,
  getRouting: () => request<RoutingRules>('GET', '/api/routing/rules'),
  putRouting: (r: RoutingRules) => request<void>('PUT', '/api/routing/rules', r),
  exportRouting: () => `${BASE}/api/routing/export`,
  importRouting: (rules: RoutingRules) => request<void>('POST', '/api/routing/import', rules),
  getRoutingTemplates: () => request<RoutingTemplate[]>('GET', '/api/routing/templates'),
  applyRoutingTemplate: (id: string) => request<void>('POST', `/api/routing/templates/${id}`),
  getProcesses: () => request<Process[]>('GET', '/api/processes'),
  // Speedtest
  speedtest: (addrs: string[]) => request<SpeedtestResult[]>('POST', '/api/speedtest', { addrs }),
  // Subscriptions
  getSubscriptions: () => request<Subscription[]>('GET', '/api/subscriptions'),
  addSubscription: (name: string, url: string) => request<Subscription>('POST', '/api/subscriptions', { name, url }),
  refreshSubscription: (id: string) => request<Subscription>('PUT', `/api/subscriptions/${id}/refresh`),
  deleteSubscription: (id: string) => request<void>('DELETE', `/api/subscriptions/${id}`),
  // Logs
  exportLogs: () => `${BASE}/api/logs/export`,
  // Stats
  getStatsHistory: (days = 7) => request<StatsHistory>('GET', `/api/stats/history?days=${days}`),
  // Backup/Restore
  backupUrl: () => `${BASE}/api/backup`,
  restore: (backup: unknown) => request<void>('POST', '/api/restore', backup),
  // Update
  checkUpdate: (force = false) => request<UpdateInfo>('GET', `/api/update/check?force=${force}`),
  getVersion: () => request<VersionInfo>('GET', '/api/version'),
  // Autostart
  getAutostart: () => request<{ enabled: boolean }>('GET', '/api/autostart'),
  setAutostart: (enabled: boolean) => request<void>('PUT', '/api/autostart', { enabled }),
  // Network/LAN
  getLanInfo: () => request<LanInfo>('GET', '/api/network/lan'),
}
