// Theme management with localStorage persistence

export type Theme = 'dark' | 'light'

function getInitialTheme(): Theme {
  if (typeof localStorage === 'undefined') return 'dark'
  const stored = localStorage.getItem('shuttle-theme') as Theme | null
  if (stored === 'dark' || stored === 'light') return stored
  // Detect system preference
  if (typeof window !== 'undefined' && window.matchMedia?.('(prefers-color-scheme: light)').matches) {
    return 'light'
  }
  return 'dark'
}

let currentTheme: Theme = getInitialTheme()
const subscribers = new Set<(theme: Theme) => void>()

function notify(): void {
  subscribers.forEach(fn => fn(currentTheme))
}

function applyTheme(theme: Theme): void {
  if (typeof document === 'undefined') return
  document.documentElement.setAttribute('data-theme', theme)
}

export function subscribeTheme(fn: (theme: Theme) => void): () => void {
  subscribers.add(fn)
  fn(currentTheme)
  return () => subscribers.delete(fn)
}

export function getTheme(): Theme {
  return currentTheme
}

export function setTheme(theme: Theme): void {
  currentTheme = theme
  localStorage.setItem('shuttle-theme', theme)
  applyTheme(theme)
  notify()
}

// Apply on load
applyTheme(currentTheme)
