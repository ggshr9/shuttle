<script lang="ts">
  import { icons, type IconName } from '@/app/icons'

  interface Props {
    name: IconName
    size?: number
    class?: string
    title?: string
  }

  let { name, size = 16, class: cls = '', title }: Props = $props()
  const def = $derived(icons[name])
  const viewBox = $derived(def?.viewBox ?? '0 0 20 20')
  const sw = $derived(def?.strokeWidth ?? 1.5)
</script>

{#if def}
  <svg
    width={size}
    height={size}
    viewBox={viewBox}
    fill="none"
    stroke="currentColor"
    stroke-width={sw}
    stroke-linecap="round"
    stroke-linejoin="round"
    class={cls}
    aria-hidden={title ? undefined : 'true'}
    aria-label={title}
    role={title ? 'img' : undefined}
  >
    {#each def.paths as p}{@html p}{/each}
  </svg>
{/if}
