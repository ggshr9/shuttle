import { describe, it, expect } from 'vitest'
import { resolveLegacyRoute } from './route-migration'

describe('resolveLegacyRoute', () => {
  it('returns null for new routes', () => {
    expect(resolveLegacyRoute('/', {})).toBeNull()
    expect(resolveLegacyRoute('/servers', {})).toBeNull()
    expect(resolveLegacyRoute('/traffic', {})).toBeNull()
  })
  it('maps /dashboard → /', () => {
    expect(resolveLegacyRoute('/dashboard', {})).toEqual({ path: '/', query: {} })
  })
  it('maps /subscriptions → /servers?source=subscriptions', () => {
    expect(resolveLegacyRoute('/subscriptions', {})).toEqual({
      path: '/servers', query: { source: 'subscriptions' },
    })
  })
  it('maps /subscriptions/:id → /servers?source=subscription:<id>', () => {
    expect(resolveLegacyRoute('/subscriptions/abc', {})).toEqual({
      path: '/servers', query: { source: 'subscription:abc' },
    })
  })
  it('maps /groups/:id → /servers?group=<id>', () => {
    expect(resolveLegacyRoute('/groups/pro', {})).toEqual({
      path: '/servers', query: { group: 'pro' },
    })
  })
  it('maps /routing → /traffic', () => {
    expect(resolveLegacyRoute('/routing', {})).toEqual({ path: '/traffic', query: {} })
  })
  it('maps /logs → /activity?tab=logs', () => {
    expect(resolveLegacyRoute('/logs', {})).toEqual({
      path: '/activity', query: { tab: 'logs' },
    })
  })
  it('preserves extra query params', () => {
    expect(resolveLegacyRoute('/subscriptions', { foo: 'bar' })).toEqual({
      path: '/servers', query: { source: 'subscriptions', foo: 'bar' },
    })
  })
})
