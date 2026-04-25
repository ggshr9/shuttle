import { createResource, createStream, type Resource, type Stream } from '@/lib/resource.svelte'
import { status as fetchStatus, getTransportStats } from '@/lib/api/endpoints'
import type { Status, TransportStats } from '@/lib/api/types'
import type { SpeedSample } from '@/lib/data/topics'

// ── Status — 3s polling (primary source of truth) ────────────
// Initial value represents a disconnected state so the UI can render before
// the first fetch resolves and stays meaningful if the backend is unreachable.
const INITIAL_STATUS: Status = { connected: false }

export function useStatus(): Resource<Status> {
  return createResource('dashboard.status', fetchStatus, {
    poll: 3000,
    initial: INITIAL_STATUS,
  })
}

// ── Transport stats — 5s polling, only while connected ───────
export function useTransportStats(): Resource<TransportStats[]> {
  return createResource(
    'dashboard.transports',
    getTransportStats,
    {
      poll: 5000,
      initial: [],
      enabled: () => useStatus().data?.connected === true,
    },
  )
}

// ── Speed stream — WebSocket push ────────────────────────────
export function useSpeedStream(): Stream<SpeedSample> {
  return createStream<SpeedSample>(
    'dashboard.speed',
    '/api/speed',
    { initial: { upload: 0, download: 0 } },
  )
}

// ── Speed history — rolling 5 min × 5s cadence = 60 samples ──
// Module-private state, shared across all callers (first-writer wins).
const MAX_POINTS = 60
let _historyInitialized = false
const _history = $state<{ up: number[]; down: number[] }>({ up: [], down: [] })

function ensureHistoryPump() {
  if (_historyInitialized) return
  _historyInitialized = true
  // Capture the stream exactly once. The registry in createStream dedupes by
  // key so both Now and Activity share the same backing state without
  // opening a second WebSocket. We never call .close() here — history is
  // an app-lifetime concern.
  const stream = useSpeedStream()
  setInterval(() => {
    if (!stream.data) return
    _history.up   = [..._history.up.slice(-(MAX_POINTS - 1)),   stream.data.upload]
    _history.down = [..._history.down.slice(-(MAX_POINTS - 1)), stream.data.download]
  }, 5000)
}

export interface SpeedHistory {
  readonly up: readonly number[]
  readonly down: readonly number[]
}
export function useSpeedHistory(): SpeedHistory {
  ensureHistoryPump()
  return {
    get up()   { return _history.up },
    get down() { return _history.down },
  }
}

// Test helper — push samples without a live WebSocket.
export function __pushHistorySample(sample: { upload: number; download: number }): void {
  _history.up   = [..._history.up.slice(-(MAX_POINTS - 1)),   sample.upload]
  _history.down = [..._history.down.slice(-(MAX_POINTS - 1)), sample.download]
}

// Test helper — clear the buffer between tests.
export function __resetHistory(): void {
  _history.up = []
  _history.down = []
}
