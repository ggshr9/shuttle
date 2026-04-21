import type { IconName } from '@/app/icons'

export interface SubNavEntry {
  slug: string
  labelKey: string
  icon: IconName
}

export const subNav: SubNavEntry[] = [
  { slug: 'general',  labelKey: 'settings.nav.general',  icon: 'settings' },
  { slug: 'proxy',    labelKey: 'settings.nav.proxy',    icon: 'servers' },
  { slug: 'mesh',     labelKey: 'settings.nav.mesh',     icon: 'mesh' },
  { slug: 'routing',  labelKey: 'settings.nav.routing',  icon: 'routing' },
  { slug: 'dns',      labelKey: 'settings.nav.dns',      icon: 'globe' },
  { slug: 'logging',  labelKey: 'settings.nav.logging',  icon: 'logs' },
  { slug: 'qos',      labelKey: 'settings.nav.qos',      icon: 'zap' },
  { slug: 'backup',   labelKey: 'settings.nav.backup',   icon: 'download' },
  { slug: 'update',   labelKey: 'settings.nav.update',   icon: 'refresh' },
  { slug: 'advanced', labelKey: 'settings.nav.advanced', icon: 'wrench' },
]

export const DEFAULT_SLUG = 'general'
