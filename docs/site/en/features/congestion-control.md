# Congestion Control

Shuttle exposes three congestion control algorithms for its QUIC-based transports (H3 and Reality). Choose based on your network conditions.

---

## Modes

### BBR

Google's Bandwidth-Based Congestion Control. BBR probes available bandwidth and maintains a model of the bottleneck link. It performs well on lossy or high-latency links where loss-based algorithms (like Cubic) over-back off.

**Best for:** General use, cloud servers with well-behaved networks.

### Brutal

Sends at a fixed, configured rate regardless of network feedback. It does not back off on loss. This is useful when you know packets are being deliberately dropped (active interference) and you want to push through at a steady rate.

**Best for:** Networks with intentional rate limiting or packet injection that causes false loss signals.

> **Warning:** Brutal is aggressive and will compete unfairly with other traffic. Use only when necessary.

### Adaptive

Monitors packet loss and RTT in real time and switches between BBR and Brutal automatically:

- Starts in **BBR** mode.
- If loss exceeds `loss_threshold` or RTT spikes above `rtt_threshold`, switches to **Brutal**.
- After `switch_cooldown` seconds without threshold violations, switches back to **BBR**.

**Best for:** Connections where conditions change — e.g. mobile networks or intermittently censored links.

---

## Configuration

```yaml
congestion:
  mode: adaptive          # bbr | brutal | adaptive

  # Brutal settings (used when mode is brutal or when adaptive switches to brutal)
  bandwidth: 50           # target send rate in Mbps

  # Adaptive thresholds
  loss_threshold: 0.02    # switch to brutal above 2% loss
  rtt_threshold: 400      # switch to brutal above 400 ms RTT
  switch_cooldown: 30     # seconds before switching back to BBR
```

### Field Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `mode` | string | `bbr` | Algorithm: `bbr` / `brutal` / `adaptive` |
| `bandwidth` | int (Mbps) | `100` | Target rate for Brutal mode |
| `loss_threshold` | float | `0.02` | Adaptive: loss rate that triggers Brutal |
| `rtt_threshold` | int (ms) | `400` | Adaptive: RTT spike that triggers Brutal |
| `switch_cooldown` | int (s) | `10` | Adaptive: seconds to wait before reverting to BBR |

---

## Switching Logic (Adaptive)

```
         loss > threshold
         or RTT > threshold
BBR ───────────────────────► Brutal
  ◄───────────────────────
         no violations for
         switch_cooldown seconds
```

The switch is per-connection. A new connection always starts in BBR mode.

---

## Which Mode to Use?

| Scenario | Recommended |
|----------|-------------|
| Standard cloud VPS | `bbr` |
| Active DPI / throttling | `brutal` |
| Mobile or variable network | `adaptive` |
| Unknown / first deployment | `adaptive` |
