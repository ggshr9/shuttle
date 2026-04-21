<script lang="ts">
  import { Empty, Card } from '@/ui'
  import { t } from '@/lib/i18n/index'
  import PeerRow from './PeerRow.svelte'
  import type { MeshPeer } from '@/lib/api/types'

  interface Props { peers: MeshPeer[] }
  let { peers }: Props = $props()
</script>

{#if peers.length === 0}
  <Card>
    <Empty
      icon="mesh"
      title={t('mesh.empty.title')}
      description={t('mesh.empty.desc')}
    />
  </Card>
{:else}
  <div class="table">
    <div class="header">
      <span></span>
      <span>{t('mesh.columns.vip')}</span>
      <span>{t('mesh.columns.state')}</span>
      <span class="num">{t('mesh.columns.method')}</span>
      <span class="num">{t('mesh.columns.rtt')}</span>
      <span class="num">{t('mesh.columns.loss')}</span>
      <span class="num">{t('mesh.columns.score')}</span>
      <span></span>
    </div>
    {#each peers as p (p.virtual_ip)}
      <PeerRow peer={p} />
    {/each}
  </div>
{/if}

<style>
  .table {
    border: 1px solid var(--shuttle-border);
    border-radius: var(--shuttle-radius-md);
    background: var(--shuttle-bg-surface);
    overflow: hidden;
  }
  .header {
    display: grid;
    grid-template-columns: 16px 160px 90px 80px 80px 80px 60px auto;
    align-items: center;
    gap: var(--shuttle-space-3);
    height: 36px;
    padding: 0 var(--shuttle-space-4);
    font-size: var(--shuttle-text-xs);
    color: var(--shuttle-fg-muted);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    background: var(--shuttle-bg-subtle);
    border-bottom: 1px solid var(--shuttle-border);
  }
  .header .num { text-align: right; }
</style>
