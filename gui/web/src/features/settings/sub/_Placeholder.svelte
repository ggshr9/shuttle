<script lang="ts">
  import { Icon } from '@/ui'
  import { t } from '@/lib/i18n/index'

  interface Props { title: string }
  let { title }: Props = $props()
</script>

<div class="placeholder">
  <Icon name="wrench" size={24} />
  <h2>{title}</h2>
  <p>{t('settings.comingNext')}</p>
</div>

<style>
  .placeholder {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    min-height: 240px;
    gap: var(--shuttle-space-3);
    color: var(--shuttle-fg-muted);
    text-align: center;
  }
  h2 {
    margin: 0;
    font-size: var(--shuttle-text-lg);
    font-weight: var(--shuttle-weight-semibold);
    color: var(--shuttle-fg-primary);
  }
  p { margin: 0; font-size: var(--shuttle-text-sm); max-width: 320px; }
</style>
