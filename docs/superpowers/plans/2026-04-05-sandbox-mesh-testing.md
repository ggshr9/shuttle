# Enhanced Sandbox: Mesh + Fault Injection + Stability Testing

> **Goal:** Enable mesh between two clients in Docker sandbox, add fault injection and stability tests, borrowing practices from Tailscale/WireGuard/Envoy.

## Industry Testing Practices Adopted

| Product | Practice | We Adopt |
|---------|----------|----------|
| **Tailscale** | Multi-node mesh with DERP relay | ✅ Two clients + server relay + P2P |
| **WireGuard** | Network namespace isolation | ✅ Docker network isolation (equivalent) |
| **Envoy** | Fault injection (abort, delay) | ✅ netem + container kill mid-connection |
| **Istio** | Connection drain verification | ✅ Migrator drain timeout tests |
| **Cloudflare** | Long-running stability | ✅ 60s sustained load test |
| **gRPC** | Connection counting/leak detection | ✅ Active conn count before/after |

## Task 1: Enable Mesh in Sandbox Configs

**Files:**
- Modify: `sandbox/configs/server.yaml` — enable mesh
- Modify: `sandbox/configs/client-a.yaml` — enable mesh + TUN
- Modify: `sandbox/configs/client-b.yaml` — enable mesh + TUN

## Task 2: Mesh E2E Tests

**File:** `test/e2e/mesh_e2e_test.go` (`//go:build sandbox`)

Tests:
- `TestSandboxMeshConnected` — both clients get mesh VIPs from server
- `TestSandboxMeshPeerDiscovery` — client-a sees client-b as peer
- `TestSandboxMeshPeerConnect` — client-a connects to client-b via P2P
- `TestSandboxMeshAPIEndpoints` — /api/mesh/status, /api/mesh/peers return data

## Task 3: Fault Injection Tests

**File:** `test/e2e/fault_test.go` (`//go:build sandbox`)

Tests borrowing from Envoy/Istio patterns:
- `TestSandboxFaultServerRestart` — kill server, verify client reconnects
- `TestSandboxFaultNetworkPartition` — iptables drop between client and server, verify circuit breaker opens, restore, verify recovery
- `TestSandboxFaultConnectionLeak` — proxy 100 requests, verify active_conns returns to 0

## Task 4: `./sandbox/run.sh dev` Command

Add a `dev` command that starts sandbox + opens browser-ready URLs:
```
./sandbox/run.sh dev
→ Starts all containers
→ Prints:
  Client A GUI: http://localhost:19091
  Client B GUI: http://localhost:19092
  Server Admin: http://localhost:19080
→ Tails logs
```
