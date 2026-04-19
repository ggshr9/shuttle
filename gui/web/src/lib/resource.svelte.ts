// createResource — reactive server-state primitive for Svelte 5 runes.
// Contract: resources with the same key share state, fetcher, and polling.
//
// NOTE: P1 does NOT auto-dispose when a component unmounts. Polling continues
// until explicit `invalidate()` or process exit. This keeps createResource
// callable from plain .svelte.ts (no component context required), simplifying
// tests. A ref-count/`$effect` dispose hook can be added in a later phase if
// polling accumulates measurably; for the current set of ≤10 resources it does
// not.

interface ResourceState<T> {
  data: T | undefined
  loading: boolean
  error: Error | null
  stale: boolean
}

interface Options<T> {
  poll?: number
  initial?: T
  enabled?: () => boolean
  onError?: (e: Error) => void
}

interface Entry<T> {
  state: ResourceState<T>
  fetcher: () => Promise<T>
  opts: Options<T>
  refCount: number
  pollTimer: ReturnType<typeof setInterval> | null
  inflight: Promise<void> | null
}

const registry = new Map<string, Entry<unknown>>()

async function runFetch<T>(entry: Entry<T>): Promise<void> {
  if (entry.inflight) return entry.inflight
  entry.state.loading = true
  const p = (async () => {
    try {
      const value = await entry.fetcher()
      entry.state.data = value
      entry.state.error = null
      entry.state.stale = false
    } catch (e) {
      entry.state.error = e instanceof Error ? e : new Error(String(e))
      entry.state.stale = true
      entry.opts.onError?.(entry.state.error)
    } finally {
      entry.state.loading = false
      entry.inflight = null
    }
  })()
  entry.inflight = p
  return p
}

function startPolling<T>(entry: Entry<T>) {
  if (!entry.opts.poll || entry.opts.poll <= 0) return
  stopPolling(entry)
  entry.pollTimer = setInterval(() => {
    if (entry.opts.enabled && !entry.opts.enabled()) return
    void runFetch(entry)
  }, entry.opts.poll)
}

function stopPolling<T>(entry: Entry<T>) {
  if (entry.pollTimer) {
    clearInterval(entry.pollTimer)
    entry.pollTimer = null
  }
}

export interface Resource<T> {
  readonly data: T | undefined
  readonly loading: boolean
  readonly error: Error | null
  readonly stale: boolean
  refetch(): Promise<void>
}

export function createResource<T>(
  key: string,
  fetcher: () => Promise<T>,
  opts: Options<T> = {},
): Resource<T> {
  let entry = registry.get(key) as Entry<T> | undefined
  if (!entry) {
    const state = $state<ResourceState<T>>({
      data: opts.initial,
      loading: false,
      error: null,
      stale: false,
    })
    entry = { state, fetcher, opts, refCount: 0, pollTimer: null, inflight: null }
    registry.set(key, entry as Entry<unknown>)
  } else {
    entry.fetcher = fetcher
    entry.opts = opts
  }
  entry.refCount++

  // Kick an initial fetch if not yet populated
  if (entry.state.data === undefined && !entry.inflight) void runFetch(entry)

  startPolling(entry)

  return {
    get data() { return entry!.state.data },
    get loading() { return entry!.state.loading },
    get error() { return entry!.state.error },
    get stale() { return entry!.state.stale },
    refetch: () => runFetch(entry!),
  }
}

export function invalidate(key: string): void {
  const entry = registry.get(key)
  if (entry) void runFetch(entry)
}

export function invalidateAll(): void {
  registry.forEach(entry => { void runFetch(entry) })
}

// Test helper — reset registry between tests
export function __resetRegistry(): void {
  registry.forEach(e => stopPolling(e))
  registry.clear()
}
