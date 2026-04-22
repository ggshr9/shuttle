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

  // Flat list of chip values in DOM order for roving-tabindex / arrow-key
  // navigation. Must stay in sync with the template below.
  const chipValues = $derived<string[]>([
    'all',
    ...sources.map((s) => s.id),
    ...groups.map((g) => `group:${g.id}`),
  ])

  // Roving tabindex — the active chip is tabbable; siblings are reachable
  // via Arrow keys per the WAI-ARIA radiogroup pattern.
  function tabIndexFor(chipValue: string): number {
    if (value === chipValue) return 0
    // If no chip is active (shouldn't happen — 'all' is always present),
    // make the first chip tabbable so users can still enter the group.
    if (!chipValues.includes(value) && chipValue === chipValues[0]) return 0
    return -1
  }

  function onKeydown(e: KeyboardEvent, chipValue: string) {
    const dir =
      e.key === 'ArrowRight' ? 1 :
      e.key === 'ArrowLeft'  ? -1 :
      e.key === 'Home'       ? 'first' :
      e.key === 'End'        ? 'last' : null
    if (dir === null) return
    e.preventDefault()

    const idx = chipValues.indexOf(chipValue)
    const nextIdx =
      dir === 'first' ? 0 :
      dir === 'last'  ? chipValues.length - 1 :
      (idx + dir + chipValues.length) % chipValues.length
    const next = chipValues[nextIdx]
    onChange(next)

    // Move focus to the new chip (query the live DOM rather than holding refs).
    queueMicrotask(() => {
      const el = document.querySelector<HTMLButtonElement>(
        `[data-chip-value="${CSS.escape(next)}"]`,
      )
      el?.focus()
    })
  }
</script>

<div class="row" role="radiogroup" aria-label="Filter by source">
  <button
    class="chip"
    class:active={value === 'all'}
    data-chip-value="all"
    onclick={() => onChange('all')}
    onkeydown={(e) => onKeydown(e, 'all')}
    role="radio"
    aria-checked={value === 'all'}
    tabindex={tabIndexFor('all')}
  >All</button>

  {#each sources as s}
    <button
      class="chip"
      class:active={value === s.id}
      data-chip-value={s.id}
      onclick={() => onChange(s.id)}
      onkeydown={(e) => onKeydown(e, s.id)}
      role="radio"
      aria-checked={value === s.id}
      tabindex={tabIndexFor(s.id)}
    >{s.label}</button>
  {/each}

  {#each groups as g}
    <button
      class="chip"
      class:active={value === `group:${g.id}`}
      data-chip-value={`group:${g.id}`}
      onclick={() => onChange(`group:${g.id}`)}
      onkeydown={(e) => onKeydown(e, `group:${g.id}`)}
      role="radio"
      aria-checked={value === `group:${g.id}`}
      tabindex={tabIndexFor(`group:${g.id}`)}
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
