import { createResource, createStream, type Resource, type Stream } from '@/lib/resource.svelte'
import { status as fetchStatus, getTransportStats } from '@/lib/api/endpoints'
import type { Status, TransportStats } from '@/lib/api/types'

// ── Status — 3s polling (primary source of truth) ────────────
export function useStatus(): Resource<Status> {
  return createResource('dashboard.status', fetchStatus, { poll: 3000 })
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
interface SpeedSample { upload: number; download: number }
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
  // Drive the buffer from the same WS stream. We don't close it — history is
  // an app-lifetime concern, not a component one.
  useSpeedStream()
  setInterval(() => {
    const s = useSpeedStream()
    if (!s.data) return
    _history.up   = [..._history.up.slice(-(MAX_POINTS - 1)),   s.data.upload]
    _history.down = [..._history.down.slice(-(MAX_POINTS - 1)), s.data.download]
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
