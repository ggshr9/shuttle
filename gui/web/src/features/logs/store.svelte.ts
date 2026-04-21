import { SvelteSet } from 'svelte/reactivity'
import { connectWS, type WSConnection } from '@/lib/ws'
import type {
  LogEntry, LogLevel, ConnDetails,
  ProtocolFilter, ActionFilter,
} from './types'

const BUFFER_CAP = 500

interface LogEvent {
  timestamp: string
  level?: LogLevel
  message?: string
}

interface ConnEvent {
  conn_id: string
  conn_state: 'opened' | 'closed'
  timestamp: string
  target: string
  protocol?: string
  rule?: string
  process_name?: string
  bytes_in?: number
  bytes_out?: number
  duration_ms?: number
}

class LogsStore {
  entries = $state<LogEntry[]>([])
  openConns = new SvelteSet<string>()

  levels = $state<SvelteSet<LogLevel>>(
    new SvelteSet(['debug', 'info', 'warn', 'error'] as LogLevel[]),
  )
  text = $state('')
  protocol = $state<ProtocolFilter>('all')
  action = $state<ActionFilter>('all')
  showConnections = $state(true)
  autoScroll = $state(true)
  selectedId = $state<string | null>(null)

  filtered = $derived.by(() => {
    const query = this.text.trim().toLowerCase()
    const levels = this.levels
    const proto = this.protocol
    const act = this.action
    const showConn = this.showConnections
    return this.entries.filter((e) => {
      if (!levels.has(e.level)) return false
      if (query && !e.msg.toLowerCase().includes(query)) return false
      if (e.kind === 'log') return true
      if (!showConn) return false
      const d = e.details!
      if (proto !== 'all' && d.protocol.toLowerCase() !== proto) return false
      if (act !== 'all' && d.rule.toLowerCase() !== act) return false
      return true
    })
  })

  selected = $derived.by(() =>
    this.selectedId
      ? this.entries.find((e) => e.id === this.selectedId) ?? null
      : null,
  )

  activeConnectionCount = $derived(this.openConns.size)

  #logWS: WSConnection | null = null
  #connWS: WSConnection | null = null
  #refCount = 0

  subscribe(): () => void {
    this.#refCount++
    if (this.#refCount === 1) {
      this.#logWS = connectWS<LogEvent>('/api/logs', (ev) => {
        this.#push({
          id: crypto.randomUUID(),
          time: Date.parse(ev.timestamp) || Date.now(),
          level: ev.level || 'info',
          msg: ev.message || '',
          kind: 'log',
        })
      })
      this.#connWS = connectWS<ConnEvent>('/api/connections', (ev) => {
        if (ev.conn_state === 'opened') {
          this.openConns.add(ev.conn_id)
          this.#push({
            id: `conn-${ev.conn_id}-open`,
            time: Date.parse(ev.timestamp) || Date.now(),
            level: 'info',
            msg: `→ ${ev.target}`,
            kind: 'conn-open',
            details: {
              connId: ev.conn_id,
              target: ev.target,
              protocol: ev.protocol || 'tcp',
              rule: ev.rule || 'default',
              process: ev.process_name || '',
              state: 'open',
            },
          })
        } else if (ev.conn_state === 'closed') {
          this.openConns.delete(ev.conn_id)
          this.#push({
            id: `conn-${ev.conn_id}-close`,
            time: Date.parse(ev.timestamp) || Date.now(),
            level: 'info',
            msg: `← ${ev.target}`,
            kind: 'conn-close',
            details: {
              connId: ev.conn_id,
              target: ev.target,
              protocol: ev.protocol || 'tcp',
              rule: ev.rule || 'default',
              process: ev.process_name || '',
              state: 'closed',
              bytesIn: ev.bytes_in ?? 0,
              bytesOut: ev.bytes_out ?? 0,
              duration: ev.duration_ms ?? 0,
            },
          })
        }
      })
    }
    return () => {
      this.#refCount--
      if (this.#refCount === 0) {
        this.#logWS?.close()
        this.#connWS?.close()
        this.#logWS = null
        this.#connWS = null
      }
    }
  }

  clear(): void {
    this.entries = []
    this.selectedId = null
  }

  toggleLevel(level: LogLevel): void {
    if (this.levels.has(level)) this.levels.delete(level)
    else this.levels.add(level)
  }

  select(id: string | null): void {
    this.selectedId = id
  }

  #push(entry: LogEntry): void {
    const next = this.entries.length >= BUFFER_CAP
      ? this.entries.slice(-(BUFFER_CAP - 1))
      : this.entries.slice()
    next.push(entry)
    this.entries = next
  }
}

export const logsStore = new LogsStore()
