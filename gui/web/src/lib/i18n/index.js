// Reactive i18n implementation for Svelte 5
import en from '../../locales/en.json'
import zhCN from '../../locales/zh-CN.json'

const locales = {
  'en': en,
  'zh-CN': zhCN,
}

// Get stored locale or detect from browser
function getInitialLocale() {
  if (typeof localStorage === 'undefined') return 'en'
  const stored = localStorage.getItem('shuttle-locale')
  if (stored && locales[stored]) return stored

  // Detect from browser
  const browserLang = navigator.language || navigator.languages?.[0]
  if (browserLang?.startsWith('zh')) return 'zh-CN'
  return 'en'
}

// Reactive locale state using a simple subscriber pattern
let currentLocale = getInitialLocale()
let subscribers = new Set()

function notify() {
  subscribers.forEach(fn => fn(currentLocale))
}

// Subscribe to locale changes
export function subscribeLocale(fn) {
  subscribers.add(fn)
  fn(currentLocale) // Call immediately with current value
  return () => subscribers.delete(fn)
}

// Get messages for current locale
function getMessages() {
  return locales[currentLocale] || locales['en']
}

// Translation function - call this within reactive context
export function t(key, params = {}) {
  const keys = key.split('.')
  let value = getMessages()

  for (const k of keys) {
    if (value && typeof value === 'object' && k in value) {
      value = value[k]
    } else {
      // Fallback to English
      value = locales['en']
      for (const fk of keys) {
        if (value && typeof value === 'object' && fk in value) {
          value = value[fk]
        } else {
          return key // Return key if not found
        }
      }
      break
    }
  }

  if (typeof value !== 'string') return key

  // Replace parameters: {name} -> params.name
  return value.replace(/\{(\w+)\}/g, (_, name) => {
    return params[name] !== undefined ? params[name] : `{${name}}`
  })
}

// Get current locale
export function getLocale() {
  return currentLocale
}

// Set locale - now reactive without page reload
export function setLocale(locale) {
  if (locales[locale] && locale !== currentLocale) {
    currentLocale = locale
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem('shuttle-locale', locale)
    }
    notify()
  }
}

// Get available locales
export function getLocales() {
  return Object.keys(locales).map(code => ({
    code,
    name: locales[code]._name || code,
  }))
}

// Create a reactive translation function for Svelte 5 components
// Usage: let translate = $derived(createTranslator(locale))
export function createTranslator(locale) {
  // Force dependency on locale parameter for reactivity
  void locale
  return t
}
