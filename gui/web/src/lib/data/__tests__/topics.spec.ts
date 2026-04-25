import { describe, it, expect } from 'vitest'
import { topicConfig, type TopicKey } from '../topics'

describe('topicConfig', () => {
  it('declares all topics in TopicMap', () => {
    const expected: TopicKey[] = ['status', 'speed', 'mesh', 'logs', 'events']
    for (const key of expected) {
      expect(topicConfig[key]).toBeDefined()
    }
  })

  it('snapshot topics omit cursorParam', () => {
    expect(topicConfig.status.kind).toBe('snapshot')
    expect((topicConfig.status as any).cursorParam).toBeUndefined()
  })

  it('stream topics declare cursorParam', () => {
    expect(topicConfig.logs.kind).toBe('stream')
    expect(topicConfig.logs.cursorParam).toBe('since')
    expect(topicConfig.events.kind).toBe('stream')
    expect(topicConfig.events.cursorParam).toBe('since')
  })

  it('every topic has positive pollMs', () => {
    for (const cfg of Object.values(topicConfig)) {
      expect(cfg.pollMs).toBeGreaterThan(0)
    }
  })
})
