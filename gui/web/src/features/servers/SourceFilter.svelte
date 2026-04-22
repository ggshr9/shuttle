<script lang="ts">
  interface Source { id: string; label: string }
  interface Group { id: string; label: string }

  interface Props {
    value: string           // 'all' | 'manual' | `subscription:<id>` | `group:<id>`
    sources: Source[]
    groups: Group[]
    onChange: (v: string) => void
  }
  let { value, sources, groups, onChange }: Props = $props()
</script>

<div class="row" role="radiogroup" aria-label="Filter by source">
  <button
    class="chip"
    class:active={value === 'all'}
    onclick={() => onChange('all')}
    role="radio"
    aria-checked={value === 'all'}
  >All</button>

  {#each sources as s}
    <button
      class="chip"
      class:active={value === s.id}
      onclick={() => onChange(s.id)}
      role="radio"
      aria-checked={value === s.id}
    >{s.label}</button>
  {/each}

  {#each groups as g}
    <button
      class="chip"
      class:active={value === `group:${g.id}`}
      onclick={() => onChange(`group:${g.id}`)}
      role="radio"
      aria-checked={value === `group:${g.id}`}
    >{g.label}</button>
  {/each}
</div>

<style>
  .row {
    display: flex; gap: var(--shuttle-space-2);
    overflow-x: auto;
    padding-bottom: var(--shuttle-space-2);
    scrollbar-width: none;
  }
  .row::-webkit-scrollbar { display: none; }
  .chip {
    flex-shrink: 0;
    padding: var(--shuttle-space-1) var(--shuttle-space-3);
    border: 1px solid var(--shuttle-border);
    border-radius: 999px;
    background: transparent;
    color: var(--shuttle-fg-secondary);
    font-size: var(--shuttle-text-sm);
    cursor: pointer;
    min-height: 36px;
    white-space: nowrap;
  }
  .chip.active {
    background: var(--shuttle-accent);
    color: var(--shuttle-accent-fg, #fff);
    border-color: var(--shuttle-accent);
  }
  .chip:focus-visible {
    outline: 2px solid var(--shuttle-accent);
    outline-offset: 2px;
  }
</style>
