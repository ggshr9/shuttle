import type { IconName } from './icons'

export type NavSection = 'overview' | 'network' | 'system'

export interface NavItem {
  id: string
  path: string
  label: string      // i18n key
  icon: IconName
  section: NavSection
  order: number
  primary: boolean   // true → appears in BottomTabs
}

export const nav: readonly NavItem[] = [
  { id: 'now',      path: '/',         label: 'nav.now',      icon: 'power',    section: 'overview', order: 10, primary: true  },
  { id: 'servers',  path: '/servers',  label: 'nav.servers',  icon: 'servers',  section: 'network',  order: 20, primary: true  },
  { id: 'traffic',  path: '/traffic',  label: 'nav.traffic',  icon: 'traffic',  section: 'network',  order: 30, primary: true  },
  { id: 'mesh',     path: '/mesh',     label: 'nav.mesh',     icon: 'mesh',     section: 'network',  order: 40, primary: true  },
  { id: 'activity', path: '/activity', label: 'nav.activity', icon: 'activity', section: 'overview', order: 50, primary: true  },
  { id: 'settings', path: '/settings', label: 'nav.settings', icon: 'settings', section: 'system',   order: 60, primary: false },
]

export function primaryNav(): NavItem[] {
  return nav.filter(n => n.primary)
}

export function navById(id: string): NavItem | undefined {
  return nav.find(n => n.id === id)
}

export function navBySection(section: NavSection): NavItem[] {
  return nav.filter(n => n.section === section).sort((a, b) => a.order - b.order)
}
