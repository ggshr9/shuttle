import { describe, it, expect, beforeEach } from 'vitest'
import { navigate, useRoute, matches, matchPath, useParams, __resetRoute } from '@/lib/router/router.svelte'

beforeEach(() => {
  location.hash = ''
  __resetRoute()
})

describe('router', () => {
  it('starts at "/"', () => {
    const r = useRoute()
    expect(r.path).toBe('/')
  })

  it('navigate() updates path', async () => {
    navigate('/servers')
    await new Promise(r => setTimeout(r, 0))
    const r = useRoute()
    expect(r.path).toBe('/servers')
    expect(location.hash).toBe('#/servers')
  })

  it('reads current hash on init', () => {
    location.hash = '#/settings/mesh'
    __resetRoute()
    const r = useRoute()
    expect(r.path).toBe('/settings/mesh')
  })

  it('matches static path', async () => {
    navigate('/servers')
    await new Promise(r => setTimeout(r, 0))
    expect(matches('/servers')).toBe(true)
    expect(matches('/groups')).toBe(false)
  })

  it('matches path with param via useParams(pattern)', async () => {
    navigate('/servers/42')
    await new Promise(r => setTimeout(r, 0))
    expect(matches('/servers/:id')).toBe(true)
    const params = useParams<{ id: string }>('/servers/:id')
    expect(params.id).toBe('42')
  })

  it('matchPath is pure — returns params without mutating state', () => {
    const result = matchPath('/users/7/posts/42', '/users/:uid/posts/:pid')
    expect(result).toEqual({ uid: '7', pid: '42' })
    expect(matchPath('/a', '/b')).toBeNull()
  })

  it('unknown path stays on it (no 404)', async () => {
    navigate('/nonexistent')
    await new Promise(r => setTimeout(r, 0))
    expect(useRoute().path).toBe('/nonexistent')
  })

  it('redirects legacy /routing to /traffic', async () => {
    localStorage.setItem('shuttle-route-migration-seen', '1')  // suppress toast
    location.hash = '#/routing'
    __resetRoute()
    await new Promise(r => setTimeout(r, 0))
    const r = useRoute()
    expect(r.path).toBe('/traffic')
    expect(location.hash).toBe('#/traffic')
  })
})
