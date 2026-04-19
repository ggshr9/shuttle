<script lang="ts">
  import type { Snippet } from 'svelte'
  import Icon from './Icon.svelte'
  import type { IconName } from '@/app/icons'

  interface Props {
    icon?: IconName
    title: string
    description?: string
    action?: Snippet
  }
  let { icon, title, description, action }: Props = $props()
</script>

<div class="empty">
  {#if icon}<div class="icon"><Icon name={icon} size={28} /></div>{/if}
  <div class="title">{title}</div>
  {#if description}<div class="desc">{description}</div>{/if}
  {#if action}<div class="action">{@render action()}</div>{/if}
</div>

<style>
  .empty {
    display: flex; flex-direction: column; align-items: center; justify-content: center;
    padding: var(--shuttle-space-7) var(--shuttle-space-5); text-align: center;
    color: var(--shuttle-fg-secondary);
  }
  .icon { color: var(--shuttle-fg-muted); margin-bottom: var(--shuttle-space-3); }
  .title { font-size: var(--shuttle-text-base); color: var(--shuttle-fg-primary); font-weight: var(--shuttle-weight-medium); }
  .desc { font-size: var(--shuttle-text-sm); color: var(--shuttle-fg-muted); margin-top: var(--shuttle-space-1); }
  .action { margin-top: var(--shuttle-space-4); }
</style>
