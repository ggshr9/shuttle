// API client for Shuttle GUI

const BASE = ''

// Auth token for API requests, injected by the Go backend at page load
// via window.__SHUTTLE_AUTH_TOKEN__ or setAuthToken().
let authToken: string = (window as any).__SHUTTLE_AUTH_TOKEN__ || ''

export function setAuthToken(token: string) {
  authToken = token
}

export function getAuthToken(): string {
  return authToken
}

interface RequestOptions extends RequestInit {
  headers: Record<string, string>
}

async function request<T>(method: string, path: string, body?: unknown, timeoutMs = 10000): Promise<T> {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), timeoutMs)
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (authToken) {
    headers['Authorization'] = `Bearer ${authToken}`
  }
  const opts: RequestOptions = {
    method,
    headers,
    signal: controller.signal,
  }
  if (body) opts.body = JSON.stringify(body)
  try {
    const res = await fetch(BASE + path, opts)
    const data = await res.json()
    if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`)
    return data as T
  } finally {
    clearTimeout(timer)
  }
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

export interface SpeedtestHistoryEntry {
  timestamp: string
  server_addr: string
  server_name?: string
  latency_ms: number
  download_bps?: number
  upload_bps?: number
  available: boolean
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

export interface DryRunResult {
  domain: string
  action: string
  matched_by: string
  rule?: string
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

export interface GeoDataStatus {
  enabled: boolean
  last_update: string
  last_error?: string
  updating: boolean
  files_present: string[]
  next_update?: string
}

export interface Status {
  connected: boolean
  server?: Server
  uptime?: number
  bytes_sent?: number
  bytes_recv?: number
}

export interface PeriodStats {
  period: string
  bytes_sent: number
  bytes_recv: number
  connections: number
  days: number
}

export interface TransportStats {
  transport: string
  active_streams: number
  total_streams: number
  bytes_sent: number
  bytes_recv: number
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
  exportRoutingData: async (): Promise<any> => {
    const res = await fetch(`${BASE}/api/routing/export`)
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    return res.json()
  },
  importRouting: (rules: RoutingRules, mode: 'merge' | 'replace' = 'merge') =>
    request<{ added: number; total: number }>('POST', '/api/routing/import', { ...rules, mode }),
  getRoutingTemplates: () => request<RoutingTemplate[]>('GET', '/api/routing/templates'),
  applyRoutingTemplate: (id: string) => request<void>('POST', `/api/routing/templates/${id}`),
  getProcesses: () => request<Process[]>('GET', '/api/processes'),
  getGeositeCategories: () => request<string[]>('GET', '/api/geosite/categories'),
  testRouting: (url: string) => request<DryRunResult>('POST', '/api/routing/test', { url }),
  // Speedtest
  speedtest: (addrs: string[]) => request<SpeedtestResult[]>('POST', '/api/speedtest', { addrs }),
  getSpeedtestHistory: (days = 30) => request<SpeedtestHistoryEntry[]>('GET', `/api/speedtest/history?days=${days}`),
  // Subscriptions
  getSubscriptions: () => request<Subscription[]>('GET', '/api/subscriptions'),
  addSubscription: (name: string, url: string) => request<Subscription>('POST', '/api/subscriptions', { name, url }),
  refreshSubscription: (id: string) => request<Subscription>('PUT', `/api/subscriptions/${id}/refresh`),
  deleteSubscription: (id: string) => request<void>('DELETE', `/api/subscriptions/${id}`),
  // Logs
  exportLogs: () => `${BASE}/api/logs/export`,
  // Stats
  getStatsHistory: (days = 7) => request<StatsHistory>('GET', `/api/stats/history?days=${days}`),
  getWeeklyStats: (weeks = 4) => request<PeriodStats[]>('GET', `/api/stats/weekly?weeks=${weeks}`),
  getMonthlyStats: (months = 6) => request<PeriodStats[]>('GET', `/api/stats/monthly?months=${months}`),
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
  // GeoData
  getGeoDataStatus: () => request<GeoDataStatus>('GET', '/api/geodata/status'),
  updateGeoData: () => request<GeoDataStatus>('POST', '/api/geodata/update', {}, 120000),
  // Transport stats
  getTransportStats: () => request<TransportStats[]>('GET', '/api/transports/stats'),
  // Diagnostics
  downloadDiagnostics: async (): Promise<void> => {
    const res = await fetch(`${BASE}/api/diagnostics`)
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    const blob = await res.blob()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    const disposition = res.headers.get('Content-Disposition')
    const match = disposition?.match(/filename="(.+)"/)
    a.download = match?.[1] || 'shuttle-diagnostics.zip'
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  },
}
