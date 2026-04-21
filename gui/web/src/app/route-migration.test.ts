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

  it('maps /groups → /servers?view=groups', () => {
    expect(resolveLegacyRoute('/groups', {})).toEqual({
      path: '/servers', query: { view: 'groups' },
    })
  })

  // Deep-path prefix matches
  it('maps /dashboard/* → / (drops tail)', () => {
    expect(resolveLegacyRoute('/dashboard/stats', {})).toEqual({ path: '/', query: {} })
  })
  it('maps /routing/rules/:id → /traffic/rules/:id (preserves tail)', () => {
    expect(resolveLegacyRoute('/routing/rules/xyz', {})).toEqual({
      path: '/traffic/rules/xyz', query: {},
    })
  })
  it('maps /logs/* → /activity?tab=logs (drops tail)', () => {
    expect(resolveLegacyRoute('/logs/filter', {})).toEqual({
      path: '/activity', query: { tab: 'logs' },
    })
  })

  // Migration tag takes precedence over incoming query of the same key
  it('migration tag wins when incoming query has conflicting key', () => {
    expect(resolveLegacyRoute('/subscriptions', { source: 'manual' })).toEqual({
      path: '/servers', query: { source: 'subscriptions' },
    })
    expect(resolveLegacyRoute('/logs', { tab: 'other' })).toEqual({
      path: '/activity', query: { tab: 'logs' },
    })
  })
})
