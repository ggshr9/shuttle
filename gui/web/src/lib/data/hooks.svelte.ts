// gui/web/src/lib/data/hooks.svelte.ts
import { getAdapter } from './index'
import type { TopicKey, TopicValue } from './topics'
import type { RequestOptions, SubscribeOptions } from './types'

export function useRequest<T = unknown>(opts: RequestOptions): Promise<T> {
  return getAdapter().request<T>(opts)
}

export function useSubscription<K extends TopicKey>(
  topic: K,
  opts?: SubscribeOptions<K>,
) {
  const adapter = getAdapter()
  // Peek cached value synchronously — does NOT open transport (no .subscribe(cb) yet).
  const initial = adapter.subscribe(topic, opts).current as TopicValue<K> | undefined
  let value = $state<TopicValue<K> | undefined>(initial)
  $effect(() => {
    const sub = adapter.subscribe(topic, opts)
    return sub.subscribe(v => { value = v as TopicValue<K> })
  })
  return {
    get value() { return value },
  }
}
