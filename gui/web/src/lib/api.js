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
  putServers: (srv) => request('PUT', '/api/config/servers', srv),
  addServer: (srv) => request('POST', '/api/config/servers', srv),
  deleteServer: (addr) => request('DELETE', '/api/config/servers', { addr }),
  getRouting: () => request('GET', '/api/routing/rules'),
  putRouting: (r) => request('PUT', '/api/routing/rules', r),
  getProcesses: () => request('GET', '/api/processes'),
}
