import { describe, it, expect } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import { useRoute, __resetRoute } from '@/lib/router/router.svelte'
import Link from '@/lib/router/Link.svelte'

describe('Link', () => {
  it('writes href to hash target', () => {
    const { container } = render(Link, { props: { to: '/servers' } })
    const a = container.querySelector('a')!
    expect(a.getAttribute('href')).toBe('#/servers')
  })

  it('updates route on click', async () => {
    location.hash = ''
    __resetRoute()
    const { container } = render(Link, { props: { to: '/mesh' } })
    const a = container.querySelector('a')!
    await fireEvent.click(a)
    await new Promise(r => setTimeout(r, 0))
    expect(useRoute().path).toBe('/mesh')
  })

  it('does not intercept when meta key held', async () => {
    location.hash = '#/'
    __resetRoute()
    const { container } = render(Link, { props: { to: '/groups' } })
    const a = container.querySelector('a')!
    await fireEvent.click(a, { metaKey: true })
    // Modifier key means browser handles the link — our handler returns early.
    // In jsdom this means location.hash is unchanged by our handler.
    // (The actual default anchor follow still happens, but test only the
    // handler boundary.)
    // If hash did update to /groups that'd still be fine, but we assert we
    // didn't mutate state via navigate().
    expect(useRoute().path).toBe('/')
  })
})
