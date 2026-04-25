import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import Diagnostics from '../Diagnostics.svelte'
import { setAdapter, __resetAdapter } from '@/lib/data'
import { Diagnostics as DiagClass } from '@/lib/data/diagnostics.svelte'
import type { DataAdapter } from '@/lib/data/types'

function makeStorage(): Storage {
  const m = new Map<string, string>()
  return {
    get length() { return m.size },
    clear() { m.clear() },
    getItem: (k) => m.get(k) ?? null,
    setItem: (k, v) => { m.set(k, v) },
    removeItem: (k) => { m.delete(k) },
    key: (i) => [...m.keys()][i] ?? null,
  }
}

function fakeAdapter(): DataAdapter {
  const diag = new DiagClass(makeStorage())
  return {
    diagnostics: diag,
    request: vi.fn(),
    subscribe: vi.fn() as any,
    connectionState: { value: 'idle', subscribe: () => () => {} },
  }
}

describe('Diagnostics.svelte', () => {
  beforeEach(() => { __resetAdapter() })
  afterEach(() => { __resetAdapter() })

  it('renders empty states when no data', () => {
    setAdapter(fakeAdapter())
    render(Diagnostics)
    expect(screen.getByText(/No errors recorded/i)).toBeInTheDocument()
    expect(screen.getByText(/No fallback events/i)).toBeInTheDocument()
  })

  it('shows — for RTT when fewer than 10 samples', () => {
    const a = fakeAdapter()
    a.diagnostics.recordRequest(20, true)
    setAdapter(a)
    render(Diagnostics)
    const dashes = screen.getAllByText('—')
    expect(dashes.length).toBeGreaterThanOrEqual(2)   // p50 + p95
  })

  it('shows lastError reason and relative time when present', () => {
    const a = fakeAdapter()
    a.diagnostics.recordRequest(50, false, 'TransportError: timeout')
    setAdapter(a)
    render(Diagnostics)
    expect(screen.getByText(/TransportError: timeout/)).toBeInTheDocument()
  })

  it('renders fallback list most-recent-first', () => {
    const a = fakeAdapter()
    a.diagnostics.recordFallback('first')
    a.diagnostics.recordFallback('second')
    setAdapter(a)
    render(Diagnostics)
    const html = document.body.innerHTML
    expect(html.indexOf('second')).toBeLessThan(html.indexOf('first'))
  })

  it('reset button clears state when confirmed', async () => {
    const a = fakeAdapter()
    a.diagnostics.recordFallback('boom')
    setAdapter(a)
    vi.spyOn(window, 'confirm').mockReturnValueOnce(true)
    render(Diagnostics)
    const btn = screen.getByRole('button', { name: /Reset counters/i })
    btn.click()
    await Promise.resolve()
    expect(a.diagnostics.snapshot().fallbacks).toHaveLength(0)
  })
})
