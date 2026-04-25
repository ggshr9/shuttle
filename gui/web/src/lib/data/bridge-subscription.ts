// gui/web/src/lib/data/bridge-subscription.ts
import { SubscriptionBase } from './subscription-base'
import type { TopicKey, TopicKind } from './topics'
import type { ConnectionStateController } from './connection-state'

export type Fetcher = (path: string) => Promise<any>

export class BridgeSubscription<T> extends SubscriptionBase<T> {
  private timer: ReturnType<typeof setTimeout> | undefined
  private stopped = true

  constructor(
    topic: TopicKey,
    kind: TopicKind,
    private readonly restPath: string,
    private readonly pollMs: number,
    private readonly cursorParam: string | undefined,
    private readonly fetcher: Fetcher,
    private readonly conn: ConnectionStateController,
  ) {
    super(topic, kind)
  }

  protected connect(): void {
    if (!this.stopped) return
    this.stopped = false
    // Schedule immediate first poll via setTimeout(0) so that it is the ONLY
    // pending timer when vi.runOnlyPendingTimersAsync() snapshots pending timers.
    // Subsequent polls are self-scheduled after each tick, so they are side-effects
    // and not included in runOnlyPendingTimersAsync's snapshot.
    this.timer = setTimeout(() => { void this.tickPoll() }, 0)
  }

  protected disconnect(): void {
    this.stopped = true
    if (this.timer !== undefined) clearTimeout(this.timer)
    this.timer = undefined
    this.inFlight = false
    this.conn.clear(this.topic)
  }

  private scheduleNext(delayMs: number): void {
    if (this.timer !== undefined) clearTimeout(this.timer)
    this.timer = setTimeout(() => {
      this.timer = undefined
      void this.tickPoll()
    }, delayMs)
  }

  private async tickPoll(): Promise<void> {
    if (this.inFlight || this.stopped) return
    this.inFlight = true
    try {
      const path = this.buildPath()
      const result = await this.fetcher(path)
      if (this.stopped) return
      this.handleSuccess(result)
    } catch (err) {
      if (this.stopped) return
      this.handleError(err)
    } finally {
      this.inFlight = false
    }
  }

  private buildPath(): string {
    if (this.kind !== 'stream' || !this.cursorParam) return this.restPath
    const sep = this.restPath.includes('?') ? '&' : '?'
    return `${this.restPath}${sep}${this.cursorParam}=${this.cursor ?? 0}`
  }

  private handleSuccess(result: any): void {
    this.errorCount = 0
    this.conn.report(this.topic, 'ok')
    if (this.kind === 'snapshot') {
      this.emit(result as T)
    } else {
      const lines = Array.isArray(result?.lines)
        ? result.lines
        : Array.isArray(result?.events)
          ? result.events
          : []
      const nextCursor = result?.cursor
      if (nextCursor !== undefined) this.cursor = nextCursor
      for (const line of lines) this.emit(line as T)
    }
    this.scheduleNext(this.pollMs)
  }

  private handleError(err: unknown): void {
    this.errorCount++
    this.conn.report(this.topic, 'error')
    // First error: retry at normal poll interval.
    // Subsequent errors: exponential backoff, capped at 30 s.
    const delay = this.errorCount === 1
      ? this.pollMs
      : Math.min(30_000, 500 * 2 ** (this.errorCount - 1))
    this.scheduleNext(delay)
  }
}
