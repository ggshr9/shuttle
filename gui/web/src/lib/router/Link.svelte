<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { HTMLAnchorAttributes } from 'svelte/elements'
  import { navigate } from './router.svelte'

  interface Props extends Omit<HTMLAnchorAttributes, 'href' | 'class' | 'children' | 'onclick'> {
    to: string
    replace?: boolean
    class?: string
    children?: Snippet
  }

  let { to, replace = false, class: cls = '', children, ...rest }: Props = $props()

  function onclick(e: MouseEvent) {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
    e.preventDefault()
    navigate(to, { replace })
  }
</script>

<a href={'#' + to} class={cls} {onclick} {...rest}>{@render children?.()}</a>
