import { describe, it, expect, vi, beforeEach } from 'vitest'
import {
  __resetResults,
  getAllResults,
  useSpeedtestResult,
  runSpeedtest,
} from '@/features/servers/resource.svelte'

vi.mock('@/lib/api/endpoints', () => ({
  getServers:       vi.fn(async () => ({ active: { addr: '' }, servers: [] })),
  addServer:        vi.fn(async () => undefined),
  setActiveServer:  vi.fn(async () => undefined),
  deleteServer:     vi.fn(async () => undefined),
  autoSelectServer: vi.fn(async () => ({ server: { addr: '' }, latency: 0 })),
  importConfig:     vi.fn(async () => ({ added: 0, total: 0 })),
  speedtest: vi.fn(async (addrs: string[]) => addrs.map((a, i) => ({
    server_addr: a,
    available: true,
    latency: 50 + i,
  }))),
}))

beforeEach(() => __resetResults())

describe('runSpeedtest', () => {
  it('stores a result per tested address', async () => {
    await runSpeedtest(['a:1', 'b:2'])
    expect(useSpeedtestResult('a:1')?.latency).toBe(50)
    expect(useSpeedtestResult('b:2')?.latency).toBe(51)
    expect(Object.keys(getAllResults())).toHaveLength(2)
  })

  it('is a no-op for empty input', async () => {
    await runSpeedtest([])
    expect(Object.keys(getAllResults())).toHaveLength(0)
  })
})
