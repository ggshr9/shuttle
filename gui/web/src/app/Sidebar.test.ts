import { describe, it, expect, beforeEach } from 'vitest'
import { render } from '@testing-library/svelte'
import { __resetRoute } from '@/lib/router/router.svelte'
import Sidebar from '@/app/Sidebar.svelte'

describe('Sidebar', () => {
  beforeEach(() => {
    location.hash = ''
    __resetRoute()
  })

  it('renders one anchor per nav item', () => {
    const { container } = render(Sidebar, { props: {} })
    const links = container.querySelectorAll('a.item')
    expect(links.length).toBe(6)
  })

  it('groups entries into three sections (overview / network / system)', () => {
    const { container } = render(Sidebar, { props: {} })
    const headings = container.querySelectorAll('.heading')
    expect(headings.length).toBe(3)
  })

  it('hides labels when collapsed', () => {
    const { container } = render(Sidebar, { props: { collapsed: true } })
    const headings = container.querySelectorAll('.heading')
    expect(headings.length).toBe(0)
  })
})
