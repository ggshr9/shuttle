import { lazy } from '@/lib/router'
import type { Component } from 'svelte'
import * as dashboard from '@/features/dashboard'
import * as servers from '@/features/servers'
import * as subscriptions from '@/features/subscriptions'
import * as groups from '@/features/groups'

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
  {
    path: '/routing',
    component: lazy(() => import('@/pages/Routing.svelte')),
    nav: { label: 'nav.routing', icon: 'routing', order: 50 },
  },
  {
    path: '/mesh',
    component: lazy(() => import('@/pages/Mesh.svelte')),
    nav: { label: 'nav.mesh', icon: 'mesh', order: 60 },
  },
  {
    path: '/logs',
    component: lazy(() => import('@/pages/Logs.svelte')),
    nav: { label: 'nav.logs', icon: 'logs', order: 80 },
  },
  {
    path: '/settings',
    component: lazy(() => import('@/pages/Settings.svelte')),
    nav: { label: 'nav.settings', icon: 'settings', order: 90 },
  },
]
