import { lazy } from '@/lib/router'
import type { Component } from 'svelte'

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
  {
    path: '/',
    component: lazy(() => import('@/pages/Dashboard.svelte')),
    nav: { label: 'nav.dashboard', icon: 'dashboard', order: 10 },
  },
  {
    path: '/servers',
    component: lazy(() => import('@/pages/Servers.svelte')),
    nav: { label: 'nav.servers', icon: 'servers', order: 20 },
  },
  {
    path: '/subscriptions',
    component: lazy(() => import('@/pages/Subscriptions.svelte')),
    nav: { label: 'nav.subscriptions', icon: 'subscriptions', order: 30 },
  },
  {
    path: '/groups',
    component: lazy(() => import('@/pages/Groups.svelte')),
    nav: { label: 'nav.groups', icon: 'groups', order: 40 },
  },
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
