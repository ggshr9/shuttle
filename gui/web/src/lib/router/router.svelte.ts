// Minimal hash-based router for Shuttle GUI.

interface RouteState {
  path: string
  query: Record<string, string>
}

const state = $state<RouteState>({ path: '/', query: {} })

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

// Pure pattern match — returns params record if the pattern matches the path,
// null otherwise. No side effects, safe to call from within $derived.
export function matchPath(path: string, pattern: string): Record<string, string> | null {
  const patternParts = pattern.split('/').filter(Boolean)
  const pathParts = path.split('/').filter(Boolean)
  if (patternParts.length !== pathParts.length) return null
  const params: Record<string, string> = {}
  for (let i = 0; i < patternParts.length; i++) {
    const p = patternParts[i]
    if (p.startsWith(':')) {
      params[p.slice(1)] = pathParts[i]
    } else if (p !== pathParts[i]) {
      return null
    }
  }
  return params
}

// Convenience against the live state. Boolean-only — does NOT expose params.
// Use `useParams(pattern)` when a component needs the extracted params.
export function matches(pattern: string): boolean {
  return matchPath(state.path, pattern) !== null
}

// Returns params for the current path under the given pattern, or an empty
// object if the path doesn't match. Safe inside $derived.
export function useParams<T extends Record<string, string>>(pattern: string): T {
  return (matchPath(state.path, pattern) ?? {}) as T
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

// Test helper — re-read location.hash
export function __resetRoute(): void {
  update()
}
