import { describe, it, expect, beforeEach } from 'vitest'
import { detect, __resetPlatform } from './index'

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
