// gui/web/src/lib/data/types.ts
import type { TopicKey, TopicValue, TopicMap } from './topics'
import type { Diagnostics } from './diagnostics.svelte'

export interface DataAdapter {
  request<T = unknown>(opts: RequestOptions): Promise<T>
  subscribe<K extends TopicKey>(
    topic: K,
    opts?: SubscribeOptions<K>,
  ): Subscription<TopicValue<K>>
  readonly connectionState: ReadableValue<ConnectionState>
  readonly diagnostics: Diagnostics
}

export type HttpMethod = 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH'

export type RequestOptions = {
  method: HttpMethod
  path: string
  body?: unknown                       // auto JSON.stringify
  headers?: Record<string, string>
  signal?: AbortSignal
  timeoutMs?: number
}

export type Subscription<T> = {
  subscribe(callback: (value: T) => void): () => void
  readonly current: T | undefined
}

export type SubscribeOptions<K extends TopicKey> = {
  cursor?: TopicMap[K]['kind'] extends 'stream' ? string | number : never
  pollInterval?: number
}

export type ConnectionState = 'idle' | 'connecting' | 'connected' | 'error'

export interface ReadableValue<T> {
  readonly value: T
  subscribe(callback: (value: T) => void): () => void
}

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string | undefined,
    message: string,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

export class TransportError extends Error {
  constructor(cause: unknown, message: string) {
    super(message, { cause })
    this.name = 'TransportError'
  }
}
