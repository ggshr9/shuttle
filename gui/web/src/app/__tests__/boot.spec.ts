import { describe, it, expect, beforeEach } from 'vitest'
import { boot } from '../boot'
import { __resetAdapter, getAdapter } from '@/lib/data'

describe('boot', () => {
  beforeEach(() => { __resetAdapter() })

  it('installs HttpAdapter when no bridge present', async () => {
    delete (window as any).ShuttleBridge
    await boot()
    expect(getAdapter()).toBeDefined()
    expect(getAdapter().connectionState.value).toBe('idle')
  })
})
