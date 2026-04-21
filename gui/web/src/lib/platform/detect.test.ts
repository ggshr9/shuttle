import { describe, it, expect, beforeEach, vi } from 'vitest'
import { detect, platform, __resetPlatform } from './index'
import * as endpoints from '../api/endpoints'

describe('detect', () => {
  beforeEach(() => {
    delete (window as any).go
    delete (window as any).ShuttleVPN
    __resetPlatform()
  })

  it('defaults to web', () => { expect(detect()).toBe('web') })

  it('detects wails when window.go.main.App present', () => {
    (window as any).go = { main: { App: {} } }
    expect(detect()).toBe('wails')
  })

  it('detects native when window.ShuttleVPN present', () => {
    (window as any).ShuttleVPN = {}
    expect(detect()).toBe('native')
  })

  it('wails takes precedence over native bridge', () => {
    (window as any).go = { main: { App: {} } }
    ;(window as any).ShuttleVPN = {}
    expect(detect()).toBe('wails')
  })
})

describe('platform proxy', () => {
  beforeEach(() => {
    delete (window as any).go
    delete (window as any).ShuttleVPN
    __resetPlatform()
    vi.restoreAllMocks()
  })

  // Regression: destructuring the proxy must preserve `this` so instance
  // methods that reference other instance methods still work.
  it('binds this even when destructured', async () => {
    const spy = vi.spyOn(endpoints, 'connect').mockResolvedValue(undefined as any)
    const { engineStart } = platform
    await engineStart()
    expect(spy).toHaveBeenCalled()
  })
})
