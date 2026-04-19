// Low-level HTTP client. Owns auth token, timeout, JSON parsing.

declare global {
  interface Window {
    __SHUTTLE_AUTH_TOKEN__?: string
  }
}

export interface ClientOptions {
  base?: string
  version?: string // reserved for future: /api/v1 vs /api/v2
  defaultTimeoutMs?: number
}

export interface Client {
  get<T>(path: string, timeoutMs?: number): Promise<T>
  post<T>(path: string, body: unknown, timeoutMs?: number): Promise<T>
  put<T>(path: string, body: unknown, timeoutMs?: number): Promise<T>
  del<T>(path: string, body?: unknown, timeoutMs?: number): Promise<T>
  raw(path: string, init?: RequestInit): Promise<Response>
  setAuthToken(token: string): void
  getAuthToken(): string
}

export function createClient(opts: ClientOptions = {}): Client {
  const base = opts.base ?? ''
  const defaultTimeout = opts.defaultTimeoutMs ?? 10000
  let authToken: string = typeof window !== 'undefined' ? (window.__SHUTTLE_AUTH_TOKEN__ ?? '') : ''

  function buildHeaders(): Record<string, string> {
    const h: Record<string, string> = { 'Content-Type': 'application/json' }
    if (authToken) h['Authorization'] = `Bearer ${authToken}`
    return h
  }

  async function request<T>(method: string, path: string, body?: unknown, timeoutMs = defaultTimeout): Promise<T> {
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), timeoutMs)
    const init: RequestInit = { method, headers: buildHeaders(), signal: controller.signal }
    if (body !== undefined) init.body = JSON.stringify(body)
    try {
      const res = await fetch(base + path, init)
      const data = await res.json().catch(() => ({}))
      if (!res.ok) throw new Error((data as { error?: string }).error || `HTTP ${res.status}`)
      return data as T
    } finally {
      clearTimeout(timer)
    }
  }

  return {
    get:  <T>(path: string, t?: number) => request<T>('GET', path, undefined, t),
    post: <T>(path: string, body: unknown, t?: number) => request<T>('POST', path, body, t),
    put:  <T>(path: string, body: unknown, t?: number) => request<T>('PUT', path, body, t),
    del:  <T>(path: string, body?: unknown, t?: number) => request<T>('DELETE', path, body, t),
    raw:  (path: string, init?: RequestInit) => {
      const headers = { ...(init?.headers ?? {}), ...(authToken ? { Authorization: `Bearer ${authToken}` } : {}) }
      return fetch(base + path, { ...init, headers })
    },
    setAuthToken: (token: string) => { authToken = token },
    getAuthToken: () => authToken,
  }
}

// Default shared client used by the app.
export const client = createClient()
export const setAuthToken = client.setAuthToken
export const getAuthToken = client.getAuthToken

// Re-exported BASE for callers that need to build URLs (downloads, SSE).
export const BASE = ''
