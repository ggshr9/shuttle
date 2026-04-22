// Shared plain-text formatter for log entries. Used by Activity page's
// Share action and (until Task 3.7 deletes it) LogsPage's Export action.
// Keeping a single source prevents the two call sites from drifting.

import type { LogEntry } from '@/features/logs/types'

export function formatLogEntry(e: LogEntry): string {
  const time = new Date(e.time).toISOString()
  let s = `[${time}] [${e.level.toUpperCase()}] ${e.msg}`
  if (e.details) {
    s += `\n  target=${e.details.target}`
    s += `\n  protocol=${e.details.protocol}`
    s += `\n  rule=${e.details.rule}`
    if (e.details.process)  s += `\n  process=${e.details.process}`
    if (e.details.duration) s += `\n  duration_ms=${e.details.duration}`
    if (e.details.bytesIn || e.details.bytesOut) {
      s += `\n  bytes_in=${e.details.bytesIn ?? 0} bytes_out=${e.details.bytesOut ?? 0}`
    }
  }
  return s
}

export function formatLogEntries(entries: readonly LogEntry[]): string {
  return entries.map(formatLogEntry).join('\n')
}
