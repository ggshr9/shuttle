<script lang="ts">
  import { Button } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import { settings } from './config.svelte'

  async function save() {
    try { await settings.save() } catch { /* toaster surfaces */ }
  }
</script>

{#if settings.isDirty}
  <div class="bar" role="status" aria-live="polite">
    <span class="msg">{t('settings.unsavedChanges')}</span>
    <div class="spacer"></div>
    <Button variant="ghost" disabled={settings.saving} onclick={() => settings.discard()}>
      {t('settings.discard')}
    </Button>
    <Button variant="primary" loading={settings.saving} onclick={save}>
      {t('settings.save')}
    </Button>
  </div>
{/if}

<style>
  .bar {
    position: sticky;
    top: 0;
    z-index: 5;
    display: flex;
    align-items: center;
    gap: var(--shuttle-space-3);
    padding: var(--shuttle-space-3) var(--shuttle-space-4);
    margin-bottom: var(--shuttle-space-4);
    background: var(--shuttle-bg-surface);
    border: 1px solid var(--shuttle-border-strong);
    border-radius: var(--shuttle-radius-md);
    box-shadow: var(--shuttle-shadow-md);
  }
  .msg {
    font-size: var(--shuttle-text-sm);
    color: var(--shuttle-fg-primary);
    font-weight: var(--shuttle-weight-medium);
  }
  .spacer { flex: 1; }
</style>
