# Test Platform Redesign — Implementation Plan

## Overview

Restructure the test platform around four principles:
1. **Transport conformance suite** — one shared test suite all transports must pass
2. **User-space network simulation** — deterministic, no Docker needed for network tests
3. **Declarative fault injection** — composable, reusable fault rules
4. **Scenario-based integration tests** — test real user stories, not just methods

## Phase 1: Transport Conformance Suite

**Goal**: Eliminate duplicated mock/fake implementations across transport packages. One suite, all transports pass it.

**Files to create**:
- `transport/conformance/suite.go` — shared test suite function
- `transport/conformance/doc.go` — package doc

**What the suite tests**:
1. `Dial` + `Accept` — basic connectivity (client dials, server accepts)
2. `OpenStream` / `AcceptStream` — bidirectional stream establishment
3. `StreamRoundTrip` — write on one side, read on the other
4. `MultiplexStreams` — open N concurrent streams, all deliver data
5. `HalfClose` — close write side, read side still works
6. `GracefulClose` — close connection, pending streams drain
7. `ConcurrentStreams` — race-detector safe parallel stream ops
8. `Backpressure` — slow reader doesn't break fast writer
9. `CancelledContext` — Dial/OpenStream with cancelled ctx returns error
10. `Type` — returns non-empty string

**API**:
```go
// transport/conformance/suite.go
package conformance

type TransportFactory func(t testing.TB) (
    client transport.ClientTransport,
    server transport.ServerTransport,
    serverAddr string,
    cleanup func(),
)

func RunSuite(t *testing.T, factory TransportFactory)
```

**Integration**: Each transport adds a thin `_conformance_test.go` that calls `conformance.RunSuite` with its own factory. For transports that can't run host-safe (need real TLS certs, etc.), the factory calls `t.Skip`.

## Phase 2: User-Space Network Simulator (`testkit/vnet`)

**Goal**: Deterministic network simulation at the `net.Conn` / stream level, without Docker or kernel TC. Build on the existing `quicfork/testutils/simnet` concepts but generalized for TCP/stream-level use.

**Files to create**:
- `testkit/vnet/link.go` — virtual link with delay, loss, bandwidth, jitter
- `testkit/vnet/node.go` — virtual network node (has address, connects via links)
- `testkit/vnet/network.go` — topology manager, creates nodes and links
- `testkit/vnet/conn.go` — `net.Conn` implementation over virtual links
- `testkit/vnet/listener.go` — `net.Listener` implementation over virtual network
- `testkit/vnet/clock.go` — pluggable clock (real or virtual for fast-forward)
- `testkit/vnet/rand.go` — seeded RNG for deterministic loss/jitter

**Core types**:
```go
type LinkConfig struct {
    Latency   time.Duration
    Jitter    time.Duration
    Loss      float64    // 0.0–1.0
    Bandwidth int64      // bytes/sec, 0 = unlimited
    Seed      int64      // deterministic RNG seed
}

type Network struct { ... }
func New(opts ...Option) *Network
func (n *Network) AddNode(name, addr string) *Node
func (n *Network) Link(a, b *Node, cfg LinkConfig)
func (n *Network) Dial(from *Node, toAddr string) (net.Conn, error)
func (n *Network) Listen(node *Node, addr string) (net.Listener, error)
```

**Key design decisions**:
- Uses `net.Pipe()` underneath, with a middleware layer applying delay/loss
- Seeded `math/rand` — same seed = same loss sequence = deterministic
- Optional virtual clock for time-acceleration (phase 2b, can defer)
- No Docker, no kernel, runs on any CI

## Phase 3: Declarative Fault Injection (`testkit/fault`)

**Goal**: Replace ad-hoc error mocks with a composable fault injection framework.

**Files to create**:
- `testkit/fault/fault.go` — core fault rule engine
- `testkit/fault/rules.go` — predefined rules (drop, delay, error, corrupt)
- `testkit/fault/conn.go` — `net.Conn` wrapper that applies fault rules
- `testkit/fault/stream.go` — `transport.Stream` wrapper that applies fault rules

**API**:
```go
type Injector struct { ... }
func New() *Injector

// Rules
func (fi *Injector) OnRead() *RuleBuilder
func (fi *Injector) OnWrite() *RuleBuilder
func (fi *Injector) OnDial() *RuleBuilder

type RuleBuilder struct { ... }
func (rb *RuleBuilder) Drop(probability float64) *RuleBuilder
func (rb *RuleBuilder) Delay(d time.Duration) *RuleBuilder
func (rb *RuleBuilder) Error(err error) *RuleBuilder
func (rb *RuleBuilder) After(d time.Duration) *RuleBuilder
func (rb *RuleBuilder) Times(n int) *RuleBuilder
func (rb *RuleBuilder) WithProbability(p float64) *RuleBuilder

// Wrappers
func (fi *Injector) WrapConn(c net.Conn) net.Conn
func (fi *Injector) WrapStream(s transport.Stream) transport.Stream
```

**Usage example**:
```go
fi := fault.New()
fi.OnRead().Delay(100 * time.Millisecond).WithProbability(0.3)
fi.OnWrite().Error(io.ErrClosedPipe).After(5 * time.Second).Times(1)

wrappedConn := fi.WrapConn(realConn)
```

## Phase 4: Scenario-Based Integration Tests

**Goal**: Test real user stories that exercise multiple components together.

**Files to create**:
- `test/scenarios/scenario.go` — scenario runner framework
- `test/scenarios/transport_fallback_test.go` — transport failover scenarios
- `test/scenarios/reconnect_test.go` — connection recovery scenarios
- `test/scenarios/congestion_switch_test.go` — adaptive CC switching scenarios

**Scenarios to implement**:

1. **Transport Fallback**: Primary transport fails mid-stream → selector falls back to secondary → in-flight data eventually delivered
2. **Reconnect on Network Change**: Connection drops (simulated via vnet link break) → client reconnects within timeout → session continues
3. **Congestion Adaptation**: Start with clean link → inject 15% loss → verify adaptive CC switches from BBR to Brutal → remove loss → verify switches back
4. **Concurrent Stream Fairness**: Open 10 streams, each transferring data → verify no stream starves (all complete within 2x of fastest)

**Pattern**: Each scenario uses `testkit/vnet` for network simulation and `testkit/fault` for failure injection. No Docker required.

## Phase 5: Performance Budget CI Gate

**Goal**: Automated performance regression detection.

**Files to create**:
- `testkit/perfbudget/budget.go` — budget definitions and checker
- `testkit/perfbudget/budget_test.go` — self-test
- `.perf-budget.yaml` — threshold definitions

**Budget format**:
```yaml
budgets:
  - name: "BenchmarkDomainTrieLookup"
    p99_ns: 500
  - name: "BenchmarkStreamCipher"
    min_throughput_mbps: 500
  - name: "BenchmarkConnectionEstablishment"
    p99_ms: 50
```

**Checker**: Parses `go test -bench -benchmem` output, compares against budgets, exits non-zero if exceeded. Integrates with existing `scripts/test.sh --bench`.

## Implementation Order

| Step | Phase | Est. Files | Dependencies |
|------|-------|-----------|--------------|
| 1    | Phase 2: vnet | 7 files | None |
| 2    | Phase 3: fault | 4 files | None (parallel with step 1) |
| 3    | Phase 1: conformance | 2 files + per-transport tests | None |
| 4    | Phase 4: scenarios | 4 files | vnet + fault |
| 5    | Phase 5: perf budget | 3 files | None |

Steps 1, 2, 3 can be developed in parallel. Step 4 depends on 1+2. Step 5 is independent.
