// Keyboard shortcuts manager

export interface ShortcutModifiers {
  ctrl?: boolean
  meta?: boolean // Command key on macOS
  shift?: boolean
  alt?: boolean
}

interface ShortcutEntry {
  callback: (e: KeyboardEvent) => void
  key: string
  ctrl: boolean
  meta: boolean
  shift: boolean
  alt: boolean
}

const shortcuts = new Map<string, ShortcutEntry>()
let initialized = false

// Register a keyboard shortcut
export function registerShortcut(
  key: string,
  callback: (e: KeyboardEvent) => void,
  options: ShortcutModifiers = {}
): () => void {
  const {
    ctrl = false,
    meta = false,
    shift = false,
    alt = false,
  } = options

  const id = buildId(key, { ctrl, meta, shift, alt })
  shortcuts.set(id, { callback, key, ctrl, meta, shift, alt })
  return () => shortcuts.delete(id)
}

function buildId(key: string, mods: ShortcutModifiers): string {
  const parts: string[] = []
  if (mods.ctrl) parts.push('ctrl')
  if (mods.meta) parts.push('meta')
  if (mods.shift) parts.push('shift')
  if (mods.alt) parts.push('alt')
  parts.push(key.toLowerCase())
  return parts.join('+')
}

function handleKeydown(e: KeyboardEvent): void {
  const id = buildId(e.key, {
    ctrl: e.ctrlKey,
    meta: e.metaKey,
    shift: e.shiftKey,
    alt: e.altKey,
  })

  const shortcut = shortcuts.get(id)
  if (shortcut) {
    e.preventDefault()
    shortcut.callback(e)
  }
}

// Initialize the shortcut listener
export function initShortcuts(): void {
  if (initialized) return
  initialized = true
  window.addEventListener('keydown', handleKeydown)
}

// Cleanup
export function destroyShortcuts(): void {
  if (!initialized) return
  initialized = false
  window.removeEventListener('keydown', handleKeydown)
  shortcuts.clear()
}

// Common shortcut presets
export const isMac = typeof navigator !== 'undefined' && /Mac/.test(navigator.platform)

// Get display text for a shortcut
export function getShortcutDisplay(key: string, mods: ShortcutModifiers = {}): string {
  const parts: string[] = []
  if (mods.ctrl) parts.push(isMac ? '^' : 'Ctrl')
  if (mods.meta) parts.push(isMac ? '\u2318' : 'Win')
  if (mods.shift) parts.push(isMac ? '\u21E7' : 'Shift')
  if (mods.alt) parts.push(isMac ? '\u2325' : 'Alt')
  parts.push(key.toUpperCase())
  return parts.join(isMac ? '' : '+')
}
