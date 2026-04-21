import type { Component } from 'svelte'
import * as dashboard from '@/features/dashboard'
import * as servers from '@/features/servers'
import * as subscriptions from '@/features/subscriptions'
import * as groups from '@/features/groups'
import * as routing from '@/features/routing'
import * as mesh from '@/features/mesh'
import * as logs from '@/features/logs'
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
  dashboard.route,
  servers.route,
  subscriptions.route,
  groups.route,
  groups.detailRoute,
  routing.route,
  mesh.route,
  logs.route,
  settings.route,
]
