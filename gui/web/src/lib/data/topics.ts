// gui/web/src/lib/data/topics.ts

// Domain types — Status and MeshPeer are imported from lib/api/types.
// SpeedSample / LogLine / EngineEvent are owned by this module since they
// describe the wire format of the corresponding subscription topics.
import type { Status, MeshPeer } from '@/lib/api/types'

export type SpeedSample = { upload: number; download: number }
export type LogLine = { ts: string; level: string; msg: string; source?: string }
export type EngineEvent = {
  cursor: number
  type: string
  data: unknown
  time: string
}

export type TopicKind = 'snapshot' | 'stream'

export type TopicEntry = {
  readonly wsPath: string
  readonly restPath: string
  readonly pollMs: number
  readonly kind: TopicKind
  readonly cursorParam?: string
}

export type TopicMap = {
  status: { value: Status; kind: 'snapshot' }
  speed: { value: SpeedSample; kind: 'snapshot' }
  mesh: { value: MeshPeer[]; kind: 'snapshot' }
  logs: { value: LogLine; kind: 'stream' }
  events: { value: EngineEvent; kind: 'stream' }
}

export type TopicKey = keyof TopicMap
export type TopicValue<K extends TopicKey> = TopicMap[K]['value']

export const topicConfig = {
  status:  { wsPath: '/ws/status',  restPath: '/api/status',     pollMs: 2000, kind: 'snapshot' },
  speed:   { wsPath: '/ws/speed',   restPath: '/api/speed',      pollMs: 1000, kind: 'snapshot' },
  mesh:    { wsPath: '/ws/mesh',    restPath: '/api/mesh/peers', pollMs: 3000, kind: 'snapshot' },
  logs:    { wsPath: '/ws/logs',    restPath: '/api/logs',       pollMs: 1000, kind: 'stream', cursorParam: 'since' },
  events:  { wsPath: '/ws/events',  restPath: '/api/events',     pollMs: 1000, kind: 'stream', cursorParam: 'since' },
} as const satisfies Record<TopicKey, TopicEntry>
