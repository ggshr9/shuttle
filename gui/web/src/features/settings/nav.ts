import type { IconName } from '@/app/icons'

export type SubNavSection = 'basics' | 'network' | 'diagnostics' | 'advanced'

export interface SubNavEntry {
  slug: string
  labelKey: string
  icon: IconName
  section: SubNavSection
}

export const subNav: SubNavEntry[] = [
  // Basics — everyday knobs
  { slug: 'general',  labelKey: 'settings.nav.general',  icon: 'settings',  section: 'basics' },
  { slug: 'proxy',    labelKey: 'settings.nav.proxy',    icon: 'servers',   section: 'basics' },
  { slug: 'update',   labelKey: 'settings.nav.update',   icon: 'refresh',   section: 'basics' },

  // Network — routing/transport stack
  { slug: 'mesh',     labelKey: 'settings.nav.mesh',     icon: 'mesh',      section: 'network' },
  { slug: 'routing',  labelKey: 'settings.nav.routing',  icon: 'routing',   section: 'network' },
  { slug: 'dns',      labelKey: 'settings.nav.dns',      icon: 'globe',     section: 'network' },

  // Diagnostics — observability
  { slug: 'diag',     labelKey: 'settings.nav.diag',     icon: 'activity',  section: 'diagnostics' },
  { slug: 'logging',  labelKey: 'settings.nav.logging',  icon: 'logs',      section: 'diagnostics' },
  { slug: 'qos',      labelKey: 'settings.nav.qos',      icon: 'zap',       section: 'diagnostics' },

  // Advanced — power user, backup/restore, tuning
  { slug: 'backup',   labelKey: 'settings.nav.backup',   icon: 'download',  section: 'advanced' },
  { slug: 'advanced', labelKey: 'settings.nav.advanced', icon: 'wrench',    section: 'advanced' },
]

export const DEFAULT_SLUG = 'general'

// Group order drives the header rendering order in SettingsPage.
export const sectionOrder: SubNavSection[] = ['basics', 'network', 'diagnostics', 'advanced']

export function entriesBySection(s: SubNavSection): SubNavEntry[] {
  return subNav.filter((e) => e.section === s)
}
