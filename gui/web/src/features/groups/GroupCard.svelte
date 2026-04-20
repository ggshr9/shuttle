<script lang="ts">
  import { Card, Badge } from '@/ui'
  import { navigate } from '@/lib/router'
  import { t } from '@/lib/i18n/index'
  import type { GroupInfo } from '@/lib/api/types'

  interface Props { group: GroupInfo }
  let { group }: Props = $props()

  function open() {
    navigate(`/groups/${encodeURIComponent(group.tag)}`)
  }
</script>

<button class="card" onclick={open} aria-label={group.tag}>
  <Card>
    <div class="top">
      <span class="tag">{group.tag}</span>
      <Badge>{group.strategy}</Badge>
    </div>
    <div class="meta">
      <div>
        <div class="label">{t('groups.members')}</div>
        <div class="val">{group.members.length}</div>
      </div>
      <div>
        <div class="label">{t('groups.selected')}</div>
        <div class="val mono">{group.selected ?? '—'}</div>
      </div>
    </div>
    <div class="spark">
      <span class="hint">{t('groups.qualityComingSoon')}</span>
    </div>
  </Card>
</button>

<style>
  .card {
    background: transparent; border: 0; padding: 0; text-align: left;
    cursor: pointer; font: inherit; color: inherit;
    display: block; width: 100%;
  }
  .top { display: flex; align-items: center; justify-content: space-between; margin-bottom: var(--shuttle-space-3); }
  .tag {
    font-family: var(--shuttle-font-mono);
    font-size: var(--shuttle-text-base);
    color: var(--shuttle-fg-primary);
    font-weight: var(--shuttle-weight-semibold);
  }
  .meta { display: grid; grid-template-columns: 1fr 1fr; gap: var(--shuttle-space-3); margin-bottom: var(--shuttle-space-3); }
  .label {
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-muted);
    text-transform: uppercase; letter-spacing: 0.06em;
  }
  .val {
    font-size: var(--shuttle-text-lg);
    color: var(--shuttle-fg-primary);
    font-variant-numeric: tabular-nums;
    margin-top: var(--shuttle-space-1);
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .val.mono { font-family: var(--shuttle-font-mono); font-size: var(--shuttle-text-sm); }
  .spark {
    height: 40px;
    display: flex; align-items: center; justify-content: center;
    background: var(--shuttle-bg-subtle);
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-sm);
  }
  .hint { font-size: var(--shuttle-text-xs); color: var(--shuttle-fg-muted); font-style: italic; }
</style>
