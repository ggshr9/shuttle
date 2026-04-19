<script lang="ts">
  import { useRoute, matches, type Lazy, type RouteDef } from './router.svelte'
  import type { Component } from 'svelte'

  interface Props {
    routes: RouteDef[]
    fallback?: Lazy<Component>
  }

  let { routes, fallback }: Props = $props()
  const route = useRoute()

  function findMatch(defs: RouteDef[], prefix = ''): Lazy<Component> | null {
    for (const d of defs) {
      const fullPath = (prefix + '/' + d.path).replace(/\/+/g, '/')
      if (matches(fullPath)) return d.component
      if (d.children) {
        const child = findMatch(d.children, fullPath)
        if (child) return child
      }
    }
    return null
  }

  // Track current path to retrigger route resolution
  const loader = $derived.by<Lazy<Component> | null>(() => {
    void route.path
    return findMatch(routes) ?? fallback ?? null
  })
</script>

{#if loader}
  {#await loader() then Loaded}
    <Loaded />
  {:catch err}
    <pre>Route load error: {String(err)}</pre>
  {/await}
{/if}
