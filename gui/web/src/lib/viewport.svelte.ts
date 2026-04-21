// Responsive viewport store. Single source of truth for viewport-driven
// layout decisions. One ResizeObserver on <html>, rAF-throttled.

export type Form = 'xs' | 'sm' | 'md' | 'lg' | 'xl'

interface Viewport {
  width: number
  form: Form
  isMobile: boolean    // xs | sm
  isTablet: boolean    // md
  isDesktop: boolean   // lg | xl
  isTouch: boolean
}

function classify(w: number): Form {
  if (w < 480)  return 'xs'
  if (w < 720)  return 'sm'
  if (w < 1024) return 'md'
  if (w < 1440) return 'lg'
  return 'xl'
}

function detectTouch(): boolean {
  if (typeof window === 'undefined' || !window.matchMedia) return false
  return window.matchMedia('(pointer: coarse)').matches
}

const initialWidth = typeof window !== 'undefined' ? window.innerWidth : 1440
const initialForm = classify(initialWidth)

export const viewport = $state<Viewport>({
  width: initialWidth,
  form: initialForm,
  isMobile: initialForm === 'xs' || initialForm === 'sm',
  isTablet: initialForm === 'md',
  isDesktop: initialForm === 'lg' || initialForm === 'xl',
  isTouch: detectTouch(),
})

function apply(w: number) {
  const f = classify(w)
  viewport.width = w
  viewport.form = f
  viewport.isMobile = f === 'xs' || f === 'sm'
  viewport.isTablet = f === 'md'
  viewport.isDesktop = f === 'lg' || f === 'xl'
  viewport.isTouch = detectTouch()
}

if (typeof window !== 'undefined' && typeof ResizeObserver !== 'undefined') {
  let pending = false
  const ro = new ResizeObserver((entries) => {
    if (pending) return
    pending = true
    requestAnimationFrame(() => {
      pending = false
      const e = entries[0]
      const w = e?.contentRect?.width ?? window.innerWidth
      apply(w)
    })
  })
  ro.observe(document.documentElement)
}

// Test helper
export function __reset(width: number = 1440): void {
  apply(width)
}
