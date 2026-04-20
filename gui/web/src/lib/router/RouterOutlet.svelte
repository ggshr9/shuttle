<script lang="ts">
  import { useRoute, matchPath, type Lazy, type RouteDef } from './router.svelte'
  import type { Component } from 'svelte'

  interface Props {
    routes: RouteDef[]
    fallback?: Lazy<Component>
  }

  let { routes, fallback }: Props = $props()
  const route = useRoute()

  function findMatch(path: string, defs: RouteDef[], prefix = ''): Lazy<Component> | null {
    for (const d of defs) {
      const fullPath = (prefix + '/' + d.path).replace(/\/+/g, '/')
      if (matchPath(path, fullPath)) return d.component
      if (d.children) {
        const child = findMatch(path, d.children, fullPath)
        if (child) return child
      }
    }
    return null
  }

  const loader = $derived.by<Lazy<Component> | null>(() => {
    const p = route.path
    return findMatch(p, routes) ?? fallback ?? null
  })
</script>

{#if loader}
  {#await loader() then Loaded}
    <Loaded />
  {:catch err}
    <pre>Route load error: {String(err)}</pre>
  {/await}
{/if}
