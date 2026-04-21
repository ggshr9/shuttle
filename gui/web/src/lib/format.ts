// Shared display formatters. Keep call sites free of ad-hoc duplicates.

// Bytes in IEC (1024-base). Used for storage and traffic counters where
// precision beats rounding to nearest decimal unit.
export function formatBytes(n: number | undefined): string {
  if (!n) return '0 B'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(2)} MB`
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`
}

// Compact HH:MM:SS clock (local time).
export function formatClock(ms: number): string {
  const d = new Date(ms)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

// Full locale timestamp with date + time, for detail views.
export function formatTimestamp(ms: number): string {
  return new Date(ms).toLocaleString()
}

// Duration in ms → human string (ms / s / m).
export function formatDuration(ms: number | undefined): string {
  if (!ms) return '0 ms'
  if (ms < 1000) return `${ms} ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(2)} s`
  return `${(ms / 60000).toFixed(2)} m`
}
