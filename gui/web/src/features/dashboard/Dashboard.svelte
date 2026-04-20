<script lang="ts">
  import { AsyncBoundary } from '@/ui'
  import { useStatus, useTransportStats } from './resource.svelte'
  import ConnectionHero from './ConnectionHero.svelte'
  import StatsGrid from './StatsGrid.svelte'
  import SpeedSparkline from './SpeedSparkline.svelte'
  import TransportBreakdown from './TransportBreakdown.svelte'

  const status = useStatus()
  const transports = useTransportStats()
</script>

<div class="page">
  <AsyncBoundary resource={status}>
    {#snippet children(st)}
      <ConnectionHero status={st} />
      <StatsGrid status={st} transports={transports.data ?? []} />
      <SpeedSparkline />
      <TransportBreakdown transports={transports.data ?? []} />
    {/snippet}
  </AsyncBoundary>
</div>

<style>
  .page {
    display: flex; flex-direction: column; gap: var(--shuttle-space-5);
    max-width: 1080px;
  }
</style>
