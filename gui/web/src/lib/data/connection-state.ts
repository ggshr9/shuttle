// gui/web/src/lib/data/connection-state.ts
import type { TopicKey } from './topics'
import type { ConnectionState, ReadableValue } from './types'

export type TopicHealth = 'ok' | 'error'

export class ConnectionStateController implements ReadableValue<ConnectionState> {
  private topicStates = new Map<TopicKey, TopicHealth>()
  private subscribers = new Set<(v: ConnectionState) => void>()
  private _value: ConnectionState = 'idle'

  get value(): ConnectionState { return this._value }

  report(topic: TopicKey, health: TopicHealth): void {
    this.topicStates.set(topic, health)
    this.recompute()
  }

  clear(topic: TopicKey): void {
    this.topicStates.delete(topic)
    this.recompute()
  }

  subscribe(callback: (value: ConnectionState) => void): () => void {
    this.subscribers.add(callback)
    callback(this._value)   // emit current immediately
    return () => { this.subscribers.delete(callback) }
  }

  private recompute(): void {
    let next: ConnectionState
    if (this.topicStates.size === 0) next = 'idle'
    else if ([...this.topicStates.values()].some(s => s === 'ok')) next = 'connected'
    else next = 'error'
    if (next !== this._value) {
      this._value = next
      for (const cb of this.subscribers) cb(next)
    }
  }
}
