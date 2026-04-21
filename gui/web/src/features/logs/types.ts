export type LogLevel = 'debug' | 'info' | 'warn' | 'error'
export type LogKind = 'log' | 'conn-open' | 'conn-close'

export interface ConnDetails {
  connId: string
  target: string
  protocol: string
  rule: string
  process: string
  state: 'open' | 'closed'
  bytesIn?: number
  bytesOut?: number
  duration?: number
}

export interface LogEntry {
  id: string
  time: number
  level: LogLevel
  msg: string
  kind: LogKind
  details?: ConnDetails
}

export type ProtocolFilter = 'all' | 'tcp' | 'udp'
export type ActionFilter = 'all' | 'proxy' | 'direct'
