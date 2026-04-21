import type { Component } from 'svelte'
import * as now from '@/features/now'
import * as servers from '@/features/servers'
import * as traffic from '@/features/traffic'
import * as mesh from '@/features/mesh'
import * as activity from '@/features/activity'
import * as settings from '@/features/settings'

export interface NavMeta {
  label: string
  icon: string
  order: number
  hidden?: boolean
}
export interface AppRoute {
  path: string
  component: () => Promise<Component>
  nav?: NavMeta
  children?: AppRoute[]
}

export const routes: AppRoute[] = [
  now.route,
  servers.route,
  traffic.route,
  mesh.route,
  activity.route,
  settings.route,
]
