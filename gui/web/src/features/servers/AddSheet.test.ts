import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'

const { endpoints, toasts, platform } = vi.hoisted(() => ({
  endpoints: {
    addServer: vi.fn<(...args: unknown[]) => Promise<void>>().mockResolvedValue(undefined),
    addSubscription: vi.fn<(...args: unknown[]) => Promise<void>>().mockResolvedValue(undefined),
    importConfig: vi.fn<(...args: unknown[]) => Promise<void>>().mockResolvedValue(undefined),
  },
  toasts: {
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
    warning: vi.fn(),
  },
  platform: {
    name: 'web' as 'web' | 'native' | 'wails',
    scanQRCode: vi.fn<() => Promise<string | 'unsupported'>>().mockResolvedValue('unsupported'),
  },
}))

vi.mock('@/lib/api/endpoints', () => endpoints)
vi.mock('@/lib/resource.svelte', () => ({ invalidate: vi.fn() }))
vi.mock('@/lib/toaster.svelte', () => ({ toasts }))
vi.mock('@/lib/platform', () => ({ platform }))

const { default: AddSheet } = await import('./AddSheet.svelte')

describe('AddSheet', () => {
  beforeEach(() => {
    endpoints.addServer.mockClear().mockResolvedValue(undefined)
    endpoints.addSubscription.mockClear().mockResolvedValue(undefined)
    endpoints.importConfig.mockClear().mockResolvedValue(undefined)
    toasts.error.mockClear(); toasts.success.mockClear()
    platform.name = 'web'
    platform.scanQRCode.mockClear().mockResolvedValue('unsupported')
  })

  it('renders all three method tabs', () => {
    const { getByText } = render(AddSheet, { props: { open: true } })
    expect(getByText('Manual')).toBeTruthy()
    expect(getByText('Paste')).toBeTruthy()
    expect(getByText('Subscribe')).toBeTruthy()
  })

  it('hides Scan QR on web runtime', () => {
    const { queryByText } = render(AddSheet, { props: { open: true } })
    expect(queryByText('Scan QR')).toBeNull()
  })

  it('switches to paste tab on click', async () => {
    const { getByText, queryByText } = render(AddSheet, { props: { open: true } })
    await fireEvent.click(getByText('Paste'))
    expect(queryByText(/Paste shuttle/)).toBeTruthy()
  })

  it('shows Scan QR button on native runtime', () => {
    platform.name = 'native'
    const { getByText } = render(AddSheet, { props: { open: true } })
    expect(getByText('Scan QR')).toBeTruthy()
  })

  // ── Submit payload paths ──────────────────────────────────────
  // Dialog portals content outside the render container, so queries
  // scan document.body via the helpers below.

  function allInputs(): HTMLInputElement[] {
    return Array.from(document.body.querySelectorAll('input')) as HTMLInputElement[]
  }
  function findButton(text: string): HTMLButtonElement {
    const btn = Array.from(document.body.querySelectorAll('button'))
      .find((b) => b.textContent?.trim() === text)
    if (!btn) throw new Error(`button "${text}" not found`)
    return btn as HTMLButtonElement
  }

  it('Manual submit calls addServer with trimmed values', async () => {
    render(AddSheet, { props: { open: true } })
    const inputs = allInputs()
    // Manual tab renders in order: addr / password / name.
    await fireEvent.input(inputs[0], { target: { value: '  example.com:443  ' } })
    await fireEvent.input(inputs[1], { target: { value: 'secret' } })
    await fireEvent.input(inputs[2], { target: { value: ' sg-01 ' } })
    await fireEvent.click(findButton('Add'))
    await Promise.resolve(); await Promise.resolve()
    expect(endpoints.addServer).toHaveBeenCalledWith({
      addr: 'example.com:443',
      password: 'secret',
      name: 'sg-01',
    })
  })

  it('Manual submit with empty addr fires error toast and skips API call', async () => {
    render(AddSheet, { props: { open: true } })
    await fireEvent.click(findButton('Add'))
    await Promise.resolve()
    expect(endpoints.addServer).not.toHaveBeenCalled()
    expect(toasts.error).toHaveBeenCalledWith('Address is required')
  })

  it('Paste submit calls importConfig with the pasted text', async () => {
    render(AddSheet, { props: { open: true } })
    await fireEvent.click(findButton('Paste'))
    const textarea = document.body.querySelector('textarea') as HTMLTextAreaElement
    await fireEvent.input(textarea, { target: { value: 'shuttle://abc@host:443' } })
    await fireEvent.click(findButton('Add'))
    await Promise.resolve(); await Promise.resolve()
    expect(endpoints.importConfig).toHaveBeenCalledWith('shuttle://abc@host:443')
    expect(endpoints.addServer).not.toHaveBeenCalled()
  })

  it('Subscribe submit calls addSubscription with the URL', async () => {
    render(AddSheet, { props: { open: true } })
    await fireEvent.click(findButton('Subscribe'))
    const input = allInputs()[0]
    await fireEvent.input(input, { target: { value: 'https://example.com/feed' } })
    await fireEvent.click(findButton('Add'))
    await Promise.resolve(); await Promise.resolve()
    expect(endpoints.addSubscription).toHaveBeenCalledWith('', 'https://example.com/feed')
  })

  it('surfaces error toast when API throws', async () => {
    endpoints.addServer.mockRejectedValue(new Error('rpc fail'))
    render(AddSheet, { props: { open: true } })
    const inputs = allInputs()
    await fireEvent.input(inputs[0], { target: { value: 'host:443' } })
    await fireEvent.click(findButton('Add'))
    await Promise.resolve(); await Promise.resolve()
    expect(toasts.error).toHaveBeenCalledWith('rpc fail')
  })
})
