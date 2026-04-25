// gui/web/src/lib/data/index.ts (initial stub — completed in Task 1.6)
import type { DataAdapter } from './types'

let _adapter: DataAdapter | null = null

export function setAdapter(a: DataAdapter): void { _adapter = a }
export function getAdapter(): DataAdapter {
  if (!_adapter) throw new Error('DataAdapter not initialised — call setAdapter() during boot')
  return _adapter
}
