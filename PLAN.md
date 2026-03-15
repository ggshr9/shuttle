# Shuttle Test Platform - Comprehensive Gap Fill Plan

## Phase 1: CI Integration (Critical)

### 1.1 Enable Go integration tests in CI
- **File**: `.github/workflows/sandbox.yml`
- Add `./sandbox/run.sh gotest` step after shell tests
- Add timeout (10 min) and proper error handling
- Ensure gotest container results are reported

## Phase 2: Network Impairment Framework (High Priority)

### 2.1 tc netem helper for Docker containers
- **New file**: `test/netem/netem.go`
- Go helper to run `tc qdisc add/change/del` via `docker exec` on containers
- Presets: `HighLatency(200ms)`, `PacketLoss(10%)`, `Bandwidth(1Mbps)`, `GFWSimulation(loss+delay)`, `Satellite(500ms+5%loss)`, `Jitter(50ms)`
- Cleanup function to remove all qdiscs

### 2.2 Congestion control under impairment tests
- **New file**: `test/e2e/netem_test.go` (sandbox build tag)
- Test BBR behavior under increasing latency (convergence)
- Test Brutal maintaining rate under packet loss
- Test Adaptive switching BBR→Brutal on interference pattern (loss + stable RTT)
- Test Adaptive staying BBR on congestion (loss + rising RTT)
- Measure throughput recovery time after impairment removal

## Phase 3: WebRTC E2E Tests (High Priority)

### 3.1 WebRTC transport E2E via sandbox
- **File**: `test/e2e/sandbox_test.go` (append)
- `TestSandboxE2EWebRTCTransport`: SOCKS5 proxy chain over WebRTC DataChannel
- `TestSandboxE2EWebRTCFallback`: WebRTC failure → H3 fallback via selector
- `TestSandboxE2EWebRTCMultiStream`: Concurrent streams over WebRTC

## Phase 4: CDN Transport Integration (Medium Priority)

### 4.1 CDN H2 stream round-trip test
- **New file**: `transport/cdn/stream_test.go`
- Start real CDN server (self-signed cert), connect H2 client, send/receive data
- Test gRPC frame encoding/decoding round-trip
- Test EOF and stream close handling
- Test large payload (>16KB) fragmentation
- Test error paths (server rejects auth, connection reset)

### 4.2 CDN E2E proxy verification
- **File**: `test/e2e/sandbox_test.go` (append)
- `TestSandboxE2ECDNConcurrent`: Multiple concurrent requests through CDN
- `TestSandboxE2ECDNLargePayload`: Transfer >1MB payload through CDN

## Phase 5: Transport Selector & Fallback (Medium Priority)

### 5.1 Selector integration tests
- **New file**: `transport/selector/integration_test.go`
- Real transport dial with first transport failing, verify fallback
- Multipath mode: verify stream distribution across paths
- Latency strategy: manipulate probe results, verify selection

### 5.2 Transport fallback E2E
- **File**: `test/e2e/sandbox_test.go` (append)
- `TestSandboxTransportFallback`: Kill H3 listener on server, verify auto-switch to Reality
- `TestSandboxTransportRecovery`: Restore H3, verify return to preferred transport

## Phase 6: Mesh/P2P Cross-NAT Tests (Medium Priority)

### 6.1 Cross-NAT hole punch test
- **File**: `mesh/p2p/sandbox_test.go` (append)
- `TestSandboxCrossNATHolePunch`: gotest on net-a punches through router to peer on net-b
- `TestSandboxmDNSDiscovery`: Verify mDNS peer discovery within same subnet
- `TestSandboxSTUNNATType`: Detect NAT type of router container

## Phase 7: Stress & Performance Tests (Medium Priority)

### 7.1 Throughput benchmarks
- **New file**: `test/bench_throughput_test.go`
- `BenchmarkH3Throughput`: Measure bytes/sec through H3 transport
- `BenchmarkRealityThroughput`: Through Reality transport
- `BenchmarkCDNThroughput`: Through CDN transport
- `BenchmarkConcurrentConnections`: 100/500/1000 concurrent SOCKS5 connections
- `BenchmarkMemoryUnderLoad`: Track allocations during sustained traffic

### 7.2 Latency percentile tests
- **New file**: `test/bench_latency_test.go`
- Measure P50/P95/P99 connection establishment latency per transport
- Measure stream open latency under load

## Phase 8: DNS Resolution Tests (Low Priority)

### 8.1 DNS integration in sandbox
- **File**: `test/e2e/sandbox_test.go` (append)
- `TestSandboxDNSResolution`: Verify DoH resolver works through proxy
- `TestSandboxDNSCache`: Verify DNS cache hit after first query
- `TestSandboxDNSPrefetch`: Verify prefetch trigger on TTL expiry

## Phase 9: Security & Crypto Validation (Low Priority)

### 9.1 Crypto validation tests
- **New file**: `test/crypto_validation_test.go`
- Verify HMAC auth rejects tampered packets
- Verify replay filter rejects duplicate packets
- Verify Noise IK handshake key binding
- Verify TLS fingerprint matches Chrome profile
- Test certificate pinning rejection on mismatch

## Phase 10: GUI API Integration (Low Priority)

### 10.1 GUI API automated tests
- **File**: `gui/api/api_test.go` (extend)
- Test WebSocket event streaming (connect, receive status updates)
- Test config update via API endpoint
- Test probe results via API
- Test concurrent API requests

## Implementation Order

1. **Phase 1** - CI fix (30 min) - Unblocks all other work
2. **Phase 4.1** - CDN stream tests (1 session) - Biggest unit test gap
3. **Phase 2** - Network impairment (1 session) - Enables congestion validation
4. **Phase 3** - WebRTC E2E (1 session) - Missing transport coverage
5. **Phase 5** - Selector/fallback (1 session) - Transport resilience
6. **Phase 6** - Cross-NAT (1 session) - Mesh coverage
7. **Phase 7** - Performance (1 session) - Baseline metrics
8. **Phase 8-10** - DNS, crypto, GUI (1 session each)
