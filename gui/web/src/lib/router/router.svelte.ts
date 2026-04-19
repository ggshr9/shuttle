// Minimal hash-based router for Shuttle GUI.

interface RouteState {
  path: string
  params: Record<string, string>
  query: Record<string, string>
}

const state = $state<RouteState>({ path: '/', params: {}, query: {} })

function parseHash(hash: string): { path: string; query: Record<string, string> } {
  let raw = hash.startsWith('#') ? hash.slice(1) : hash
  if (!raw) raw = '/'
  const [path, qs] = raw.split('?')
  const query: Record<string, string> = {}
  if (qs) {
    new URLSearchParams(qs).forEach((v, k) => { query[k] = v })
  }
  return { path: path || '/', query }
}

function update() {
  const { path, query } = parseHash(location.hash)
  state.path = path
  state.query = query
  state.params = {} // re-derived by matches()
}

if (typeof window !== 'undefined') {
  window.addEventListener('hashchange', update)
  update()
}

export function navigate(path: string, opts: { replace?: boolean } = {}): void {
  const hash = '#' + (path.startsWith('/') ? path : '/' + path)
  if (opts.replace) {
    history.replaceState(null, '', hash)
    update()
  } else {
    location.hash = hash
  }
}

export function useRoute(): Readonly<RouteState> {
  return state
}

export function useParams<T extends Record<string, string>>(): T {
  return state.params as T
}

// Returns true + populates params if pattern matches current state.path.
export function matches(pattern: string): boolean {
  const patternParts = pattern.split('/').filter(Boolean)
  const pathParts = state.path.split('/').filter(Boolean)
  if (patternParts.length !== pathParts.length) return false
  const params: Record<string, string> = {}
  for (let i = 0; i < patternParts.length; i++) {
    const p = patternParts[i]
    if (p.startsWith(':')) {
      params[p.slice(1)] = pathParts[i]
    } else if (p !== pathParts[i]) {
      return false
    }
  }
  state.params = params
  return true
}

export type Lazy<T> = () => Promise<T>
export function lazy<T>(loader: () => Promise<{ default: T }>): Lazy<T> {
  return async () => (await loader()).default
}

import type { Component } from 'svelte'

export interface RouteDef {
  path: string
  component: Lazy<Component>
  children?: RouteDef[]
}

// Test helper — re-read location.hash (also usable by production code to force
// a sync after manually manipulating location).
export function __resetRoute(): void {
  update()
}
