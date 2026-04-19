export type Theme = 'dark' | 'light'

function readInitial(): Theme {
  try {
    const stored = localStorage?.getItem?.('shuttle-theme')
    if (stored === 'dark' || stored === 'light') return stored
  } catch {
    // localStorage may be unavailable (sandboxed iframe, SSR, etc.)
  }
  try {
    if (window.matchMedia?.('(prefers-color-scheme: light)').matches) return 'light'
  } catch {
    // matchMedia may be unavailable
  }
  return 'dark'
}

function apply(theme: Theme) {
  try {
    document.documentElement.setAttribute('data-theme', theme)
  } catch {
    // document may be unavailable
  }
}

const state = $state<{ theme: Theme }>({ theme: readInitial() })
apply(state.theme)

export const theme = {
  get current(): Theme { return state.theme },
  set(next: Theme) {
    state.theme = next
    try {
      localStorage?.setItem?.('shuttle-theme', next)
    } catch {
      // ignore
    }
    apply(next)
  },
  toggle() { this.set(state.theme === 'dark' ? 'light' : 'dark') },
}
