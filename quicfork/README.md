# quicfork — Local Fork of quic-go

This directory contains a local fork of [quic-go](https://github.com/quic-go/quic-go)
with a minimal patch for pluggable congestion control.

## What's Changed

- **`congestion_hook.go`** (new): Exports a `CongestionControl` interface that wraps
  quic-go's internal congestion controller types, enabling BBR/Brutal/Adaptive integration.
- **`connection.go`**: Two call sites pass `Config.CongestionController` to the ack handler.
- **`interface.go`**: `Config` struct gains a `CongestionController CongestionControl` field.

## Maintenance Policy

1. **Track upstream releases**: Run `scripts/quicfork-check.sh` to see divergence.
2. **Security patches**: Monitor quic-go releases and CVEs. Apply security fixes promptly.
3. **Minimize divergence**: Only modify files necessary for the CC hook. Do not add
   other features to the fork.
4. **Upgrade process**:
   - Update upstream version in `go.mod`
   - Copy new upstream source into `quicfork/`
   - Re-apply the 3 modifications (congestion_hook.go, connection.go, interface.go)
   - Run `scripts/quicfork-check.sh` to verify minimal divergence
   - Run full test suite
