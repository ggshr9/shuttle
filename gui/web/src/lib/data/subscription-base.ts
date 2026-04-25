// gui/web/src/lib/data/subscription-base.ts
import type { TopicKey, TopicKind } from './topics'

export abstract class SubscriptionBase<T> {
  protected subscribers = new Set<(v: T) => void>()
  protected currentValue: T | undefined
  protected lastHash: string | undefined
  protected cursor: string | number | undefined
  protected errorCount = 0
  protected inFlight = false
  private paused = false

  constructor(
    protected readonly topic: TopicKey,
    protected readonly kind: TopicKind,
  ) {}

  /** Open the underlying connection / start the timer. */
  protected abstract connect(): void
  /** Tear down the underlying connection / clear the timer. */
  protected abstract disconnect(): void
  /** Execute one fetch cycle (poll) — for subclasses that need it. Default no-op. */
  protected async tick(): Promise<void> { /* no-op */ }

  get current(): T | undefined { return this.kind === 'snapshot' ? this.currentValue : undefined }

  add(callback: (v: T) => void): () => void {
    const wasEmpty = this.subscribers.size === 0
    this.subscribers.add(callback)
    if (wasEmpty) {
      this.connect()
    } else if (this.kind === 'snapshot' && this.currentValue !== undefined) {
      const cached = this.currentValue
      queueMicrotask(() => {
        if (this.subscribers.has(callback)) callback(cached)
      })
    }
    return () => {
      if (!this.subscribers.delete(callback)) return
      if (this.subscribers.size === 0) this.disconnect()
    }
  }

  /** Subclasses call this to deliver a value to subscribers. Snapshot kind diffs by JSON hash. */
  protected emit(value: T): void {
    if (this.kind === 'snapshot') {
      const hash = JSON.stringify(value)
      if (hash === this.lastHash) return
      this.lastHash = hash
      this.currentValue = value
    }
    for (const cb of [...this.subscribers]) cb(value)
  }

  pauseForHidden(): void {
    if (this.paused) return
    this.paused = true
    if (this.subscribers.size > 0) this.disconnect()
  }

  resumeFromHidden(): void {
    if (!this.paused) return
    this.paused = false
    if (this.subscribers.size > 0) this.connect()
  }
}
