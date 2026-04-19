// Feature route registry — P2 and later phases add entries.
import type { Lazy } from '@/lib/router'
import type { Component } from 'svelte'

export interface NavMeta {
  label: string
  icon: string
  order: number
  hidden?: boolean
}

export interface AppRoute {
  path: string
  component: Lazy<Component>
  nav?: NavMeta
  children?: AppRoute[]
}

export const routes: AppRoute[] = [
  // Populated in P2+ via feature index.ts imports.
]
