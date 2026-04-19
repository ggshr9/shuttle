<script lang="ts">
  interface Props { size?: number }
  let { size = 16 }: Props = $props()
</script>

<span
  class="sp"
  style={`width:${size}px;height:${size}px;border-width:${Math.max(1, Math.round(size / 10))}px`}
  role="status"
  aria-label="Loading"
></span>

<style>
  .sp {
    display: inline-block;
    border: 2px solid var(--shuttle-border);
    border-top-color: var(--shuttle-fg-primary);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }
  @keyframes spin { to { transform: rotate(360deg); } }
</style>
