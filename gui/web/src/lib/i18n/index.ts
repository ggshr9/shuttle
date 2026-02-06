// Reactive i18n implementation for Svelte 5
import en from '../../locales/en.json'
import zhCN from '../../locales/zh-CN.json'

type LocaleCode = 'en' | 'zh-CN'
type Messages = typeof en
type TranslateParams = Record<string, string | number>

const locales: Record<LocaleCode, Messages> = {
  'en': en,
  'zh-CN': zhCN as Messages,
}

// Get stored locale or detect from browser
function getInitialLocale(): LocaleCode {
  if (typeof localStorage === 'undefined') return 'en'
  const stored = localStorage.getItem('shuttle-locale') as LocaleCode | null
  if (stored && stored in locales) return stored

  // Detect from browser
  const browserLang = navigator.language || navigator.languages?.[0]
  if (browserLang?.startsWith('zh')) return 'zh-CN'
  return 'en'
}

// Reactive locale state using a simple subscriber pattern
let currentLocale: LocaleCode = getInitialLocale()
const subscribers = new Set<(locale: LocaleCode) => void>()

function notify(): void {
  subscribers.forEach(fn => fn(currentLocale))
}

// Subscribe to locale changes
export function subscribeLocale(fn: (locale: LocaleCode) => void): () => void {
  subscribers.add(fn)
  fn(currentLocale) // Call immediately with current value
  return () => subscribers.delete(fn)
}

// Get messages for current locale
function getMessages(): Messages {
  return locales[currentLocale] || locales['en']
}

// Translation function - call this within reactive context
export function t(key: string, params: TranslateParams = {}): string {
  const keys = key.split('.')
  let value: unknown = getMessages()

  for (const k of keys) {
    if (value && typeof value === 'object' && k in value) {
      value = (value as Record<string, unknown>)[k]
    } else {
      // Fallback to English
      value = locales['en'] as unknown
      for (const fk of keys) {
        if (value && typeof value === 'object' && fk in value) {
          value = (value as Record<string, unknown>)[fk]
        } else {
          return key // Return key if not found
        }
      }
      break
    }
  }

  if (typeof value !== 'string') return key

  // Replace parameters: {name} -> params.name
  return value.replace(/\{(\w+)\}/g, (_, name: string) => {
    return params[name] !== undefined ? String(params[name]) : `{${name}}`
  })
}

// Get current locale
export function getLocale(): LocaleCode {
  return currentLocale
}

// Set locale - now reactive without page reload
export function setLocale(locale: LocaleCode): void {
  if (locale in locales && locale !== currentLocale) {
    currentLocale = locale
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem('shuttle-locale', locale)
    }
    notify()
  }
}

export interface LocaleInfo {
  code: LocaleCode
  name: string
}

// Get available locales
export function getLocales(): LocaleInfo[] {
  return (Object.keys(locales) as LocaleCode[]).map(code => ({
    code,
    name: (locales[code] as Record<string, unknown>)._name as string || code,
  }))
}

// Create a reactive translation function for Svelte 5 components
// Usage: let translate = $derived(createTranslator(locale))
export function createTranslator(locale: string): typeof t {
  // Force dependency on locale parameter for reactivity
  void locale
  return t
}
