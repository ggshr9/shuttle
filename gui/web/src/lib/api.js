const BASE = ''

async function request(method, path, body) {
  const opts = {
    method,
    headers: { 'Content-Type': 'application/json' },
  }
  if (body) opts.body = JSON.stringify(body)
  const res = await fetch(BASE + path, opts)
  const data = await res.json()
  if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`)
  return data
}

export const api = {
  status: () => request('GET', '/api/status'),
  connect: () => request('POST', '/api/connect'),
  disconnect: () => request('POST', '/api/disconnect'),
  getConfig: () => request('GET', '/api/config'),
  putConfig: (cfg) => request('PUT', '/api/config', cfg),
  getServers: () => request('GET', '/api/config/servers'),
  setActiveServer: (srv) => request('PUT', '/api/config/servers', srv),
  addServer: (srv) => request('POST', '/api/config/servers', srv),
  deleteServer: (addr) => request('DELETE', '/api/config/servers', { addr }),
  autoSelectServer: () => request('POST', '/api/config/servers/auto-select'),
  importConfig: (data) => request('POST', '/api/config/import', { data }),
  exportConfig: (format = 'json') => `${BASE}/api/config/export?format=${format}`,
  getRouting: () => request('GET', '/api/routing/rules'),
  putRouting: (r) => request('PUT', '/api/routing/rules', r),
  exportRouting: () => `${BASE}/api/routing/export`,
  importRouting: (rules) => request('POST', '/api/routing/import', rules),
  getRoutingTemplates: () => request('GET', '/api/routing/templates'),
  applyRoutingTemplate: (id) => request('POST', `/api/routing/templates/${id}`),
  getProcesses: () => request('GET', '/api/processes'),
  // Speedtest
  speedtest: (addrs) => request('POST', '/api/speedtest', { addrs }),
  // Subscriptions
  getSubscriptions: () => request('GET', '/api/subscriptions'),
  addSubscription: (name, url) => request('POST', '/api/subscriptions', { name, url }),
  refreshSubscription: (id) => request('PUT', `/api/subscriptions/${id}/refresh`),
  deleteSubscription: (id) => request('DELETE', `/api/subscriptions/${id}`),
  // Logs
  exportLogs: () => `${BASE}/api/logs/export`,
  // Stats
  getStatsHistory: (days = 7) => request('GET', `/api/stats/history?days=${days}`),
  // Backup/Restore
  backupUrl: () => `${BASE}/api/backup`,
  restore: (backup) => request('POST', '/api/restore', backup),
  // Update
  checkUpdate: (force = false) => request('GET', `/api/update/check?force=${force}`),
  getVersion: () => request('GET', '/api/version'),
  // Autostart
  getAutostart: () => request('GET', '/api/autostart'),
  setAutostart: (enabled) => request('PUT', '/api/autostart', { enabled }),
  // Network/LAN
  getLanInfo: () => request('GET', '/api/network/lan'),
}
