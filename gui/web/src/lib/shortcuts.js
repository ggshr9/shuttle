// Keyboard shortcuts manager

const shortcuts = new Map()
let initialized = false

// Register a keyboard shortcut
export function registerShortcut(key, callback, options = {}) {
  const {
    ctrl = false,
    meta = false, // Command key on macOS
    shift = false,
    alt = false,
  } = options

  const id = buildId(key, { ctrl, meta, shift, alt })
  shortcuts.set(id, { callback, key, ctrl, meta, shift, alt })
  return () => shortcuts.delete(id)
}

function buildId(key, mods) {
  const parts = []
  if (mods.ctrl) parts.push('ctrl')
  if (mods.meta) parts.push('meta')
  if (mods.shift) parts.push('shift')
  if (mods.alt) parts.push('alt')
  parts.push(key.toLowerCase())
  return parts.join('+')
}

function handleKeydown(e) {
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
export function initShortcuts() {
  if (initialized) return
  initialized = true
  window.addEventListener('keydown', handleKeydown)
}

// Cleanup
export function destroyShortcuts() {
  if (!initialized) return
  initialized = false
  window.removeEventListener('keydown', handleKeydown)
  shortcuts.clear()
}

// Common shortcut presets
export const isMac = typeof navigator !== 'undefined' && /Mac/.test(navigator.platform)

// Get display text for a shortcut
export function getShortcutDisplay(key, mods = {}) {
  const parts = []
  if (mods.ctrl) parts.push(isMac ? '^' : 'Ctrl')
  if (mods.meta) parts.push(isMac ? '⌘' : 'Win')
  if (mods.shift) parts.push(isMac ? '⇧' : 'Shift')
  if (mods.alt) parts.push(isMac ? '⌥' : 'Alt')
  parts.push(key.toUpperCase())
  return parts.join(isMac ? '' : '+')
}
