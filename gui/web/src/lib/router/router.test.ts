import { describe, it, expect, beforeEach } from 'vitest'
import { navigate, useRoute, matches, __resetRoute } from '@/lib/router/router.svelte'

beforeEach(() => {
  __resetRoute()
  location.hash = ''
})

describe('router', () => {
  it('starts at "/"', () => {
    const r = useRoute()
    expect(r.path).toBe('/')
  })

  it('navigate() updates path', async () => {
    navigate('/servers')
    // hashchange is async in jsdom
    await new Promise(r => setTimeout(r, 0))
    const r = useRoute()
    expect(r.path).toBe('/servers')
    expect(location.hash).toBe('#/servers')
  })

  it('reads current hash on init', () => {
    location.hash = '#/settings/mesh'
    __resetRoute()
    // __resetRoute clears state but init reads location.hash
    const r = useRoute()
    expect(r.path).toBe('/settings/mesh')
  })

  it('matches static path', async () => {
    navigate('/servers')
    await new Promise(r => setTimeout(r, 0))
    expect(matches('/servers')).toBe(true)
    expect(matches('/groups')).toBe(false)
  })

  it('matches path with param', async () => {
    navigate('/groups/42')
    await new Promise(r => setTimeout(r, 0))
    expect(matches('/groups/:id')).toBe(true)
    const r = useRoute()
    expect(r.params.id).toBe('42')
  })

  it('unknown path stays on it (no 404)', async () => {
    navigate('/nonexistent')
    await new Promise(r => setTimeout(r, 0))
    expect(useRoute().path).toBe('/nonexistent')
  })
})
