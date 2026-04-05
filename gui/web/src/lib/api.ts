// API client for Shuttle GUI

declare global {
  interface Window {
    __SHUTTLE_AUTH_TOKEN__?: string
  }
}

const BASE = ''

// Auth token for API requests, injected by the Go backend at page load
// via window.__SHUTTLE_AUTH_TOKEN__ or setAuthToken().
let authToken: string = window.__SHUTTLE_AUTH_TOKEN__ || ''

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
  sni?: string
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

export interface PathStats {
  interface: string
  local_addr: string
  rtt_ms: number
  loss_rate: number
  bytes_sent: number
  bytes_recv: number
  available: boolean
}

export interface ConnectionHistoryEntry {
  id: string
  timestamp: string
  target: string
  rule: string
  protocol: string
  process_name?: string
  bytes_in: number
  bytes_out: number
  duration_ms: number
  state: string
}

export interface StreamInfo {
  stream_id: number
  conn_id: string
  target: string
  transport: string
  bytes_sent: number
  bytes_received: number
  errors: number
  closed: boolean
  duration_ms: number
}

export interface DebugState {
  engine_state: string
  circuit_breaker: string
  streams: number
  transport: string
  uptime_seconds: number
  goroutines: number
}

export interface SystemResources {
  goroutines: number
  mem_alloc_mb: number
  mem_sys_mb: number
  mem_gc_cycles: number
  num_cpu: number
  uptime_seconds: number
}

export interface RoutingConflict {
  domain: string
  action1: string
  action2: string
  rule1: string
  rule2: string
}

export interface ProbeResult {
  success: boolean
  status?: number
  status_text?: string
  via: string
  latency_ms: number
  error?: string
  headers?: Record<string, string[]>
  body?: string
}

export interface BatchProbeResult {
  name: string
  url: string
  via: string
  success: boolean
  status?: number
  latency_ms: number
  error?: string
  body?: string
}

export interface ConfigValidation {
  valid: boolean
  errors: string[]
}

export interface GeodataSourcePreset {
  id: string
  name: string
  description: string
  direct_list: string
  proxy_list: string
  reject_list: string
  gfw_list: string
  cn_cidr: string
  private_cidr: string
}

export interface MeshStatus {
  enabled: boolean
  virtual_ip?: string
  cidr?: string
  peer_count?: number
}

export interface MeshPeer {
  virtual_ip: string
  state: string
  method?: string
  avg_rtt_ms?: number
  packet_loss?: number
  score?: number
}

export const api = {
  status: () => request<Status>('GET', '/api/status'),
  connect: () => request<void>('POST', '/api/connect'),
  disconnect: () => request<void>('POST', '/api/disconnect'),
  getConfig: () => request<Config>('GET', '/api/config'),
  putConfig: (cfg: Config) => request<void>('PUT', '/api/config', cfg),
  getServers: () => request<ServersResponse>('GET', '/api/config/servers'),
  setActiveServer: (srv: Server) => request<void>('PUT', '/api/config/servers', srv),
  /** @deprecated Use setActiveServer instead */
  putServers: (srv: Server) => request<void>('PUT', '/api/config/servers', srv),
  addServer: (srv: Server) => request<void>('POST', '/api/config/servers', srv),
  deleteServer: (addr: string) => request<void>('DELETE', '/api/config/servers', { addr }),
  autoSelectServer: () => request<AutoSelectResult>('POST', '/api/config/servers/auto-select'),
  importConfig: (data: string) => request<ImportResult>('POST', '/api/config/import', { data }),
  exportConfig: (format = 'json') => `${BASE}/api/config/export?format=${format}`,
  getRouting: () => request<RoutingRules>('GET', '/api/routing/rules'),
  putRouting: (r: RoutingRules) => request<void>('PUT', '/api/routing/rules', r),
  exportRouting: () => `${BASE}/api/routing/export`,
  exportRoutingData: () => request<RoutingRules>('GET', '/api/routing/export'),
  importRouting: (rules: RoutingRules, mode: 'merge' | 'replace' = 'merge') =>
    request<{ added: number; total: number }>('POST', '/api/routing/import', { ...rules, mode }),
  getRoutingTemplates: () => request<RoutingTemplate[]>('GET', '/api/routing/templates'),
  applyRoutingTemplate: (id: string) => request<void>('POST', `/api/routing/templates/${id}`),
  getProcesses: () => request<Process[]>('GET', '/api/processes'),
  getGeositeCategories: () => request<string[]>('GET', '/api/geosite/categories'),
  testRouting: (url: string) => request<DryRunResult>('POST', '/api/routing/test', { url }),
  // Speedtest
  speedtest: async (addrs: string[]): Promise<SpeedtestResult[]> => {
    const data = await request<{ results: SpeedtestResult[] }>('POST', '/api/speedtest', { addrs })
    return data.results
  },
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
  // Multipath stats
  getMultipathStats: () => request<PathStats[]>('GET', '/api/multipath/stats'),
  // Connection history
  getConnectionsHistory: () => request<ConnectionHistoryEntry[]>('GET', '/api/connections/history'),
  // Streams by connection ID
  getConnectionStreams: (id: string) => request<StreamInfo[]>('GET', `/api/connections/${id}/streams`),
  // Debug state
  getDebugState: () => request<DebugState>('GET', '/api/debug/state'),
  // System resources
  getSystemResources: () => request<SystemResources>('GET', '/api/system/resources'),
  // PAC script
  getPacScript: () => request<string>('GET', '/api/pac'),
  // Routing conflicts
  getRoutingConflicts: () => request<{ conflicts: RoutingConflict[]; count: number }>('GET', '/api/routing/conflicts'),
  // Config validation
  validateConfig: (config: Config) => request<ConfigValidation>('POST', '/api/config/validate', config),
  // Test probe
  testProbe: (url: string, via?: string) => request<ProbeResult>('POST', '/api/test/probe', { url, via }, 20000),
  // Test probe batch
  testProbeBatch: (tests: Array<{ name?: string; url: string; method?: string; via?: string }>) =>
    request<{ results: BatchProbeResult[] }>('POST', '/api/test/probe/batch', { tests }, 60000),
  // Geodata sources
  getGeodataSources: () => request<GeodataSourcePreset[]>('GET', '/api/geodata/sources'),
  // Update geodata source preset
  updateGeodataSource: (id: string) => request<{ status: string; source: string }>('POST', `/api/geodata/sources/${id}`),
  // Mesh VPN
  meshStatus: () => request<MeshStatus>('GET', '/api/mesh/status'),
  meshPeers: () => request<MeshPeer[]>('GET', '/api/mesh/peers'),
  meshConnectPeer: (vip: string) => request<void>('POST', `/api/mesh/peers/${encodeURIComponent(vip)}/connect`),
  // Diagnostics
  downloadDiagnostics: async (): Promise<void> => {
    const headers: Record<string, string> = {}
    if (authToken) {
      headers['Authorization'] = `Bearer ${authToken}`
    }
    const res = await fetch(`${BASE}/api/diagnostics`, { headers })
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
