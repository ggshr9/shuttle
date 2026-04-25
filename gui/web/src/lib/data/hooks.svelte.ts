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
  let value = $state<TopicValue<K> | undefined>(undefined)
  const adapter = getAdapter()
  const sub = adapter.subscribe(topic, opts)
  // Initial replay of cached snapshot value, if any.
  value = sub.current as TopicValue<K> | undefined
  $effect(() => {
    return sub.subscribe(v => { value = v as TopicValue<K> })
  })
  return {
    get value() { return value },
  }
}
