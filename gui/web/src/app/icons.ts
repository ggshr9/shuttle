// Icon registry — maps semantic name → inline SVG path data.
// Add new icons here; reference via <Icon name="..."/> from ui/Icon.svelte.
// All icons: 20x20 viewBox, stroke 1.5, currentColor, no fill.

export interface IconPath {
  paths: string[]       // each entry is a <path d="..."/> or similar element as SVG text
  viewBox?: string      // default "0 0 20 20"
  strokeWidth?: number  // default 1.5
}

export const icons = {
  dashboard: {
    paths: [
      '<rect x="3" y="3" width="6" height="6" rx="1"/>',
      '<rect x="11" y="3" width="6" height="6" rx="1"/>',
      '<rect x="3" y="11" width="6" height="6" rx="1"/>',
      '<rect x="11" y="11" width="6" height="6" rx="1"/>',
    ],
  },
  servers: {
    paths: [
      '<rect x="3" y="3" width="14" height="5" rx="1.5"/>',
      '<rect x="3" y="12" width="14" height="5" rx="1.5"/>',
      '<circle cx="6" cy="5.5" r="1" fill="currentColor"/>',
      '<circle cx="6" cy="14.5" r="1" fill="currentColor"/>',
    ],
  },
  subscriptions: {
    paths: ['<path d="M4 5h12M4 10h12M4 15h8"/>', '<circle cx="16" cy="15" r="2"/>'],
  },
  groups: {
    paths: [
      '<circle cx="10" cy="5" r="2"/>',
      '<circle cx="4" cy="15" r="2"/>',
      '<circle cx="10" cy="15" r="2"/>',
      '<circle cx="16" cy="15" r="2"/>',
      '<path d="M10 7v5M10 12l-6 1M10 12l6 1"/>',
    ],
  },
  routing: {
    paths: [
      '<circle cx="5" cy="10" r="2"/>',
      '<circle cx="15" cy="5" r="2"/>',
      '<circle cx="15" cy="15" r="2"/>',
      '<path d="M7 10h3l2-5h1M10 10l2 5h1"/>',
    ],
  },
  mesh: {
    paths: [
      '<circle cx="10" cy="4" r="2"/>',
      '<circle cx="3" cy="15" r="2"/>',
      '<circle cx="17" cy="15" r="2"/>',
      '<path d="M10 6v3M5 14l4-5M15 14l-4-5M5 15h10"/>',
    ],
  },
  logs: {
    paths: [
      '<path d="M5 4h10a1 1 0 011 1v10a1 1 0 01-1 1H5a1 1 0 01-1-1V5a1 1 0 011-1z"/>',
      '<path d="M7 8h6M7 11h4"/>',
    ],
  },
  settings: {
    paths: [
      '<circle cx="10" cy="10" r="3"/>',
      '<path d="M10 3v2M10 15v2M3 10h2M15 10h2M5.05 5.05l1.41 1.41M13.54 13.54l1.41 1.41M5.05 14.95l1.41-1.41M13.54 6.46l1.41-1.41"/>',
    ],
  },
  check:        { paths: ['<path d="M5 10l3 3 7-7"/>'] },
  x:            { paths: ['<path d="M5 5l10 10M15 5l-10 10"/>'] },
  chevronRight: { paths: ['<path d="M8 5l5 5-5 5"/>'] },
  chevronLeft:  { paths: ['<path d="M12 5l-5 5 5 5"/>'] },
  chevronDown:  { paths: ['<path d="M5 8l5 5 5-5"/>'] },
  plus:         { paths: ['<path d="M10 4v12M4 10h12"/>'] },
  trash:        { paths: ['<path d="M4 6h12M7 6V4h6v2M6 6l1 10h6l1-10"/>'] },
  info:         { paths: ['<circle cx="10" cy="10" r="7"/>', '<path d="M10 9v4M10 6v.01"/>'] },
} satisfies Record<string, IconPath>

export type IconName = keyof typeof icons
