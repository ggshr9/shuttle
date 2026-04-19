// All backend types. Pure declarations — no runtime code.
// When adding a new type, group with related ones.

// ── Servers ──────────────────────────────────────────────────
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

export interface ImportResult {
  added: number
  total: number
  servers?: Server[]
  errors?: string[]
  error?: string
}

export interface AutoSelectResult {
  server: Server
  latency: number
}

// ── Config ───────────────────────────────────────────────────
export interface Config {
  server?: Server
  servers?: Server[]
  proxy: {
    system_proxy?: {
      enabled: boolean
    }
  }
}

export interface ConfigValidation {
  valid: boolean
  errors: string[]
}

// ── Status ───────────────────────────────────────────────────
export interface Status {
  connected: boolean
  server?: Server
  uptime?: number
  bytes_sent?: number
  bytes_recv?: number
}

// ── Subscriptions ────────────────────────────────────────────
export interface Subscription {
  id: string
  name: string
  url: string
  servers?: Server[]
  updated_at?: string
  error?: string
}

// ── Speedtest ────────────────────────────────────────────────
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

// ── Routing ──────────────────────────────────────────────────
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

export interface DryRunResult {
  domain: string
  action: string
  matched_by: string
  rule?: string
}

export interface RoutingConflict {
  domain: string
  action1: string
  action2: string
  rule1: string
  rule2: string
}

// ── Processes / System ───────────────────────────────────────
export interface Process {
  name: string
  conns: number
}

// ── Stats ────────────────────────────────────────────────────
export interface StatsHistory {
  [date: string]: {
    upload: number
    download: number
  }
}

export interface PeriodStats {
  period: string
  bytes_sent: number
  bytes_recv: number
  connections: number
  days: number
}

// ── Updates / version ────────────────────────────────────────
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

// ── Network ──────────────────────────────────────────────────
export interface LanInfo {
  ip: string
  gateway: string
  interfaces: string[]
}

// ── GeoData ──────────────────────────────────────────────────
export interface GeoDataStatus {
  enabled: boolean
  last_update: string
  last_error?: string
  updating: boolean
  files_present: string[]
  next_update?: string
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

// ── Transports / paths ───────────────────────────────────────
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

// ── Connections / streams ────────────────────────────────────
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

// ── Debug / diagnostics ──────────────────────────────────────
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

// ── Mesh ─────────────────────────────────────────────────────
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

// ── Groups ───────────────────────────────────────────────────
export interface GroupInfo {
  tag: string
  strategy: string
  members: string[]
  selected?: string
  latencies?: Record<string, number>
}

export interface GroupTestResult {
  tag: string
  latency_ms: number
  available: boolean
}
