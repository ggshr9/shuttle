// Endpoint functions grouped by feature. All go through `client`.

import { client, BASE } from './client'
import type {
  Status,
  Server, ServersResponse, Config, ImportResult, AutoSelectResult, ConfigValidation,
  RoutingRules, RoutingTemplate, DryRunResult, RoutingConflict,
  Process,
  SpeedtestResult, SpeedtestHistoryEntry,
  Subscription,
  StatsHistory, PeriodStats,
  UpdateInfo, VersionInfo,
  LanInfo,
  GeoDataStatus, GeodataSourcePreset,
  TransportStats, PathStats,
  ConnectionHistoryEntry, StreamInfo,
  DebugState, SystemResources,
  ProbeResult, BatchProbeResult,
  MeshStatus, MeshPeer,
  GroupInfo, GroupTestResult,
} from './types'

// ── Engine / status ──────────────────────────────────────────
export const status = () => client.get<Status>('/api/status')
export const connect = () => client.post<void>('/api/connect', {})
export const disconnect = () => client.post<void>('/api/disconnect', {})

// ── Config ───────────────────────────────────────────────────
export const getConfig = () => client.get<Config>('/api/config')
export const putConfig = (cfg: Config) => client.put<void>('/api/config', cfg)
export const importConfig = (data: string) => client.post<ImportResult>('/api/config/import', { data })
export const exportConfig = (format = 'json') => `${BASE}/api/config/export?format=${format}`
export const validateConfig = (cfg: Config) => client.post<ConfigValidation>('/api/config/validate', cfg)

// ── Servers ──────────────────────────────────────────────────
export const getServers = () => client.get<ServersResponse>('/api/config/servers')
export const setActiveServer = (srv: Server) => client.put<void>('/api/config/servers', srv)
/** @deprecated Use setActiveServer instead */
export const putServers = setActiveServer
export const addServer = (srv: Server) => client.post<void>('/api/config/servers', srv)
export const deleteServer = (addr: string) => client.del<void>('/api/config/servers', { addr })
export const autoSelectServer = () => client.post<AutoSelectResult>('/api/config/servers/auto-select', {})

// ── Routing ──────────────────────────────────────────────────
export const getRouting = () => client.get<RoutingRules>('/api/routing/rules')
export const putRouting = (r: RoutingRules) => client.put<void>('/api/routing/rules', r)
export const exportRouting = () => `${BASE}/api/routing/export`
export const exportRoutingData = () => client.get<RoutingRules>('/api/routing/export')
export const importRouting = (rules: RoutingRules, mode: 'merge' | 'replace' = 'merge') =>
  client.post<{ added: number; total: number }>('/api/routing/import', { ...rules, mode })
export const getRoutingTemplates = () => client.get<RoutingTemplate[]>('/api/routing/templates')
export const applyRoutingTemplate = (id: string) => client.post<void>(`/api/routing/templates/${id}`, {})
export const getGeositeCategories = () => client.get<string[]>('/api/geosite/categories')
export const testRouting = (url: string) => client.post<DryRunResult>('/api/routing/test', { url })
export const getRoutingConflicts = () =>
  client.get<{ conflicts: RoutingConflict[]; count: number }>('/api/routing/conflicts')
export const getPacScript = () => client.get<string>('/api/pac')

// ── Processes ────────────────────────────────────────────────
export const getProcesses = () => client.get<Process[]>('/api/processes')

// ── Speedtest ────────────────────────────────────────────────
export const speedtest = async (addrs: string[]): Promise<SpeedtestResult[]> => {
  const data = await client.post<{ results: SpeedtestResult[] }>('/api/speedtest', { addrs })
  return data.results
}
export const getSpeedtestHistory = (days = 30) =>
  client.get<SpeedtestHistoryEntry[]>(`/api/speedtest/history?days=${days}`)

// ── Subscriptions ────────────────────────────────────────────
export const getSubscriptions = () => client.get<Subscription[]>('/api/subscriptions')
export const addSubscription = (name: string, url: string) =>
  client.post<Subscription>('/api/subscriptions', { name, url })
export const refreshSubscription = (id: string) =>
  client.put<Subscription>(`/api/subscriptions/${id}/refresh`, {})
export const deleteSubscription = (id: string) =>
  client.del<void>(`/api/subscriptions/${id}`)

// ── Logs ─────────────────────────────────────────────────────
export const exportLogs = () => `${BASE}/api/logs/export`

// ── Stats ────────────────────────────────────────────────────
export const getStatsHistory = (days = 7) =>
  client.get<StatsHistory>(`/api/stats/history?days=${days}`)
export const getWeeklyStats = (weeks = 4) =>
  client.get<PeriodStats[]>(`/api/stats/weekly?weeks=${weeks}`)
export const getMonthlyStats = (months = 6) =>
  client.get<PeriodStats[]>(`/api/stats/monthly?months=${months}`)

// ── Backup / restore ─────────────────────────────────────────
export const backupUrl = () => `${BASE}/api/backup`
export const restore = (backup: unknown) => client.post<void>('/api/restore', backup)

// ── Update / version ─────────────────────────────────────────
export const checkUpdate = (force = false) => client.get<UpdateInfo>(`/api/update/check?force=${force}`)
export const getVersion = () => client.get<VersionInfo>('/api/version')

// ── Autostart ────────────────────────────────────────────────
export const getAutostart = () => client.get<{ enabled: boolean }>('/api/autostart')
export const setAutostart = (enabled: boolean) =>
  client.put<void>('/api/autostart', { enabled })

// ── Network ──────────────────────────────────────────────────
export const getLanInfo = () => client.get<LanInfo>('/api/network/lan')

// ── GeoData ──────────────────────────────────────────────────
export const getGeoDataStatus = () => client.get<GeoDataStatus>('/api/geodata/status')
export const updateGeoData = () => client.post<GeoDataStatus>('/api/geodata/update', {}, 120000)
export const getGeodataSources = () => client.get<GeodataSourcePreset[]>('/api/geodata/sources')
export const updateGeodataSource = (id: string) =>
  client.post<{ status: string; source: string }>(`/api/geodata/sources/${id}`, {})

// ── Transport / multipath ────────────────────────────────────
export const getTransportStats = () => client.get<TransportStats[]>('/api/transports/stats')
export const getMultipathStats = () => client.get<PathStats[]>('/api/multipath/stats')

// ── Connections / streams ────────────────────────────────────
export const getConnectionsHistory = () =>
  client.get<ConnectionHistoryEntry[]>('/api/connections/history')
export const getConnectionStreams = (id: string) =>
  client.get<StreamInfo[]>(`/api/connections/${id}/streams`)

// ── Debug / system ───────────────────────────────────────────
export const getDebugState = () => client.get<DebugState>('/api/debug/state')
export const getSystemResources = () => client.get<SystemResources>('/api/system/resources')

// ── Probe / test ─────────────────────────────────────────────
export const testProbe = (url: string, via?: string) =>
  client.post<ProbeResult>('/api/test/probe', { url, via }, 20000)
export const testProbeBatch = (tests: Array<{ name?: string; url: string; method?: string; via?: string }>) =>
  client.post<{ results: BatchProbeResult[] }>('/api/test/probe/batch', { tests }, 60000)

// ── Mesh ─────────────────────────────────────────────────────
export const meshStatus = () => client.get<MeshStatus>('/api/mesh/status')
export const meshPeers = () => client.get<MeshPeer[]>('/api/mesh/peers')
export const meshConnectPeer = (vip: string) =>
  client.post<void>(`/api/mesh/peers/${encodeURIComponent(vip)}/connect`, {})

// ── Outbound groups ──────────────────────────────────────────
export const getGroups = () => client.get<GroupInfo[]>('/api/groups')
export const getGroup = (tag: string) =>
  client.get<GroupInfo>(`/api/groups/${encodeURIComponent(tag)}`)
export const testGroup = (tag: string) =>
  client.post<GroupTestResult[]>(`/api/groups/${encodeURIComponent(tag)}/test`, {})
export const selectGroupMember = (groupTag: string, member: string) =>
  client.put<void>(`/api/groups/${encodeURIComponent(groupTag)}/selected`, { selected: member })

// ── Diagnostics (binary download) ────────────────────────────
export const downloadDiagnostics = async (): Promise<void> => {
  const res = await client.raw('/api/diagnostics')
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
}

// ── Aggregate (legacy compat) ────────────────────────────────
// `api.xxx(...)` call sites in legacy pages continue to work via this object.
// New code should import the individual fn by name.
export const api = {
  status, connect, disconnect,
  getConfig, putConfig, importConfig, exportConfig, validateConfig,
  getServers, setActiveServer, putServers, addServer, deleteServer, autoSelectServer,
  getRouting, putRouting, exportRouting, exportRoutingData, importRouting,
  getRoutingTemplates, applyRoutingTemplate, getGeositeCategories, testRouting, getRoutingConflicts,
  getPacScript,
  getProcesses,
  speedtest, getSpeedtestHistory,
  getSubscriptions, addSubscription, refreshSubscription, deleteSubscription,
  exportLogs,
  getStatsHistory, getWeeklyStats, getMonthlyStats,
  backupUrl, restore,
  checkUpdate, getVersion,
  getAutostart, setAutostart,
  getLanInfo,
  getGeoDataStatus, updateGeoData, getGeodataSources, updateGeodataSource,
  getTransportStats, getMultipathStats,
  getConnectionsHistory, getConnectionStreams,
  getDebugState, getSystemResources,
  testProbe, testProbeBatch,
  meshStatus, meshPeers, meshConnectPeer,
  getGroups, getGroup, testGroup, selectGroupMember,
  downloadDiagnostics,
}
