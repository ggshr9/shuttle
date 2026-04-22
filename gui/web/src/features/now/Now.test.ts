import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'

// Mutable mocks hoisted alongside vi.mock() so test bodies can swap
// runtime name + permission result between cases.
const { platformMock, toastErrors } = vi.hoisted(() => ({
  platformMock: {
    name: 'web' as 'web' | 'native' | 'wails',
    engineStart: vi.fn<() => Promise<void>>().mockResolvedValue(undefined),
    engineStop: vi.fn<() => Promise<void>>().mockResolvedValue(undefined),
    engineStatus: vi.fn().mockResolvedValue({ connected: false, server: null }),
    requestVpnPermission: vi.fn<() => Promise<'granted' | 'denied' | 'unsupported'>>()
      .mockResolvedValue('unsupported'),
    onStatusChange: () => () => {},
  },
  toastErrors: [] as string[],
}))

vi.mock('@/lib/platform', () => ({ platform: platformMock }))

vi.mock('@/lib/resources/status.svelte', () => ({
  useStatus: () => ({
    data: { connected: false, uptime: 0, server: null },
    loading: false,
    error: undefined,
    refetch: () => {},
  }),
  useSpeedStream: () => ({ data: { download: 0, upload: 0 } }),
  useTransportStats: () => ({ data: [] }),
}))

vi.mock('@/lib/toaster.svelte', () => ({
  toasts: {
    error: (m: string) => { toastErrors.push(m) },
    success: () => {}, info: () => {}, warning: () => {},
  },
}))

// Now.svelte imports must come after vi.mock() calls so the mocked modules
// resolve during the module graph walk.
const { default: Now } = await import('./Now.svelte')

describe('Now', () => {
  beforeEach(() => {
    platformMock.name = 'web'
    platformMock.engineStart.mockClear().mockResolvedValue(undefined)
    platformMock.engineStop.mockClear().mockResolvedValue(undefined)
    platformMock.requestVpnPermission.mockClear().mockResolvedValue('unsupported')
    toastErrors.length = 0
  })
  afterEach(() => { vi.clearAllTimers() })

  it('renders a power button', () => {
    const { container } = render(Now)
    expect(container.querySelector('[role="switch"]')).toBeTruthy()
  })

  it('web runtime: tap calls engineStart without checking permission', async () => {
    const { container } = render(Now)
    await fireEvent.click(container.querySelector('[role="switch"]')!)
    expect(platformMock.requestVpnPermission).not.toHaveBeenCalled()
    expect(platformMock.engineStart).toHaveBeenCalled()
  })

  it('native + granted: tap triggers permission then engineStart', async () => {
    platformMock.name = 'native'
    platformMock.requestVpnPermission.mockResolvedValue('granted')
    const { container } = render(Now)
    await fireEvent.click(container.querySelector('[role="switch"]')!)
    // microtask drain
    await Promise.resolve(); await Promise.resolve()
    expect(platformMock.requestVpnPermission).toHaveBeenCalled()
    expect(platformMock.engineStart).toHaveBeenCalled()
  })

  it('native + denied: tap shows error toast and does NOT call engineStart', async () => {
    platformMock.name = 'native'
    platformMock.requestVpnPermission.mockResolvedValue('denied')
    const { container } = render(Now)
    await fireEvent.click(container.querySelector('[role="switch"]')!)
    await Promise.resolve(); await Promise.resolve()
    expect(platformMock.requestVpnPermission).toHaveBeenCalled()
    expect(platformMock.engineStart).not.toHaveBeenCalled()
    expect(toastErrors).toContain('VPN permission denied')
  })

  it('native + unsupported: falls through to engineStart (graceful degrade)', async () => {
    platformMock.name = 'native'
    platformMock.requestVpnPermission.mockResolvedValue('unsupported')
    const { container } = render(Now)
    await fireEvent.click(container.querySelector('[role="switch"]')!)
    await Promise.resolve(); await Promise.resolve()
    expect(platformMock.engineStart).toHaveBeenCalled()
    expect(toastErrors).toEqual([])
  })

  it('surfaces a toast when engineStart throws', async () => {
    platformMock.engineStart.mockRejectedValue(new Error('rpc fail'))
    const { container } = render(Now)
    await fireEvent.click(container.querySelector('[role="switch"]')!)
    await Promise.resolve(); await Promise.resolve()
    expect(toastErrors).toContain('rpc fail')
  })

  it('handles string throw via errorMessage normalizer', async () => {
    platformMock.engineStart.mockRejectedValue('bridge offline')
    const { container } = render(Now)
    await fireEvent.click(container.querySelector('[role="switch"]')!)
    await Promise.resolve(); await Promise.resolve()
    expect(toastErrors).toContain('bridge offline')
  })
})
