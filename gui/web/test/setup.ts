import '@testing-library/jest-dom/vitest'

// jsdom does not ship ResizeObserver — stub it for components that use it.
if (typeof ResizeObserver === 'undefined') {
  ;(globalThis as any).ResizeObserver = class ResizeObserver {
    observe()    {}
    unobserve()  {}
    disconnect() {}
  }
}

// Stub matchMedia for theme.svelte tests
if (!window.matchMedia) {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }),
  })
}

// jsdom under some configs ships a localStorage global that is not a full
// Storage — methods missing. Replace with an in-memory shim for tests.
if (typeof localStorage === 'undefined' || typeof localStorage.getItem !== 'function') {
  const mem = new Map<string, string>()
  const storage: Storage = {
    get length() { return mem.size },
    clear: () => { mem.clear() },
    getItem: (k: string) => mem.get(k) ?? null,
    key: (i: number) => Array.from(mem.keys())[i] ?? null,
    removeItem: (k: string) => { mem.delete(k) },
    setItem: (k: string, v: string) => { mem.set(k, String(v)) },
  }
  Object.defineProperty(globalThis, 'localStorage', { value: storage, writable: true, configurable: true })
  Object.defineProperty(globalThis, 'sessionStorage', { value: storage, writable: true, configurable: true })
}
