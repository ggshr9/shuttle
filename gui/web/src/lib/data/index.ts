// gui/web/src/lib/data/index.ts
import type { DataAdapter } from './types'

let _adapter: DataAdapter | null = null

export function setAdapter(a: DataAdapter): void { _adapter = a }

export function getAdapter(): DataAdapter {
  if (!_adapter) throw new Error('DataAdapter not initialised — call setAdapter() during boot')
  return _adapter
}

/** Returns null instead of throwing — for early-boot paths that may run before setAdapter. */
export function tryGetAdapter(): DataAdapter | null {
  return _adapter
}

/** Test-only helper. */
export function __resetAdapter(): void { _adapter = null }
