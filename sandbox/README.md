# Shuttle Sandbox Test Environment

Docker-based isolated testing environment for Shuttle.
**All tests run inside Docker — zero impact on host network.**

## Architecture

```
┌── Your Mac Browser ─────────────────────────────────────────┐
│  http://localhost:5174  (Svelte GUI via Vite)                │
│       ↓ /api/* proxy                                         │
├──────────────────────────────────────────────────────────────┤
│                    Docker Compose Sandbox                     │
├──────────────────────────────────────────────────────────────┤
│                                                               │
│   ┌─────────────┐                      ┌─────────────┐       │
│   │  client-a   │                      │  client-b   │       │
│   │  (shuttle)  │                      │  (shuttle)  │       │
│   │ 10.100.1.10 │                      │ 10.100.2.10 │       │
│   │  :19091 API │                      │  :19092 API │       │
│   └──────┬──────┘                      └──────┬──────┘       │
│          │  net-a                    net-b    │               │
│          │  10.100.1.0/24      10.100.2.0/24  │               │
│   ┌──────┴────────────────────────────────────┴──────┐       │
│   │                     router                        │       │
│   │              (NAT + Firewall)                     │       │
│   │                   10.100.0.1                      │       │
│   └──────────────────────┬───────────────────────────┘       │
│                   net-server                                  │
│                   10.100.0.0/24                               │
│          ┌───────────┼───────────┐                            │
│   ┌──────┴──────┐ ┌──┴──┐ ┌─────┴─────┐                     │
│   │   server    │ │stun │ │  httpbin  │                      │
│   │ (shuttled)  │ │     │ │ (target)  │                      │
│   │ 10.100.0.10 │ │ .30 │ │   .20     │                      │
│   └─────────────┘ └─────┘ └───────────┘                      │
└──────────────────────────────────────────────────────────────┘
```

## Quick Start

```bash
# Full automated test
./run.sh

# GUI development mode (browser-based testing)
./run.sh gui
```

## GUI Testing (Browser)

Test the full GUI + proxy chain from your browser, with zero host network impact:

```bash
# 1. Start sandbox with GUI API exposed
./run.sh gui

# 2. In another terminal, start the frontend
cd gui/web && npm run dev:sandbox

# 3. Open browser
open http://localhost:5174
```

Now you can:
- Click **Connect/Disconnect** — controls the Docker client
- **Import servers** via shuttle:// URI
- **Switch transports** (H3/Reality)
- **View logs** in real-time (WebSocket)
- **Speed test** servers
- **Test probe** — send requests through the full proxy chain

### Test Probe API (Full Chain Verification)

```bash
# Single request through SOCKS5 proxy
curl -s localhost:19091/api/test/probe \
  -d '{"url":"http://10.100.0.20/ip","via":"socks5"}' | jq .

# Batch test — SOCKS5 + HTTP + Direct comparison
curl -s localhost:19091/api/test/probe/batch -d '{
  "tests": [
    {"name":"socks5", "url":"http://10.100.0.20/ip", "via":"socks5"},
    {"name":"http",   "url":"http://10.100.0.20/ip", "via":"http"},
    {"name":"direct", "url":"http://10.100.0.20/ip", "via":"direct"}
  ]
}' | jq .

# Test Client B
curl -s localhost:19092/api/test/probe \
  -d '{"url":"http://10.100.0.20/ip","via":"socks5"}' | jq .
```

### Two-Client Testing

Switch between clients to test different perspectives:

```bash
# Client A GUI (default)
npm run dev:sandbox

# Client B GUI
npm run dev:sandbox-b
```

## Commands

| Command | Description |
|---------|-------------|
| `./run.sh` | Run everything (build, start, test, stop) |
| `./run.sh gui` | Start sandbox + show GUI connection instructions |
| `./run.sh build` | Build binaries and Docker images |
| `./run.sh up` | Start the sandbox environment |
| `./run.sh down` | Stop the sandbox environment |
| `./run.sh test` | Run automated test suite |
| `./run.sh gotest` | Run Go integration tests in Docker |
| `./run.sh logs` | View container logs |
| `./run.sh shell server` | Open shell in container |
| `./run.sh clean` | Clean up everything |

## Port Mapping

| Host Port | Container | Description |
|-----------|-----------|-------------|
| 19091 | client-a:9090 | Client A GUI API |
| 19092 | client-b:9090 | Client B GUI API |
| 19443 | server:443 | Server (H3/QUIC) |
| 19080 | server:9090 | Server Admin API |

## Network Simulation

Simulate poor network conditions via the router container:

```bash
# Add 100ms latency
docker compose exec router tc qdisc add dev eth0 root netem delay 100ms

# Add 5% packet loss
docker compose exec router tc qdisc add dev eth0 root netem loss 5%

# Limit bandwidth to 1Mbps
docker compose exec router tc qdisc add dev eth0 root tbf rate 1mbit burst 32kbit latency 400ms

# Remove all rules
docker compose exec router tc qdisc del dev eth0 root
```

## Requirements

- Docker + Docker Compose v2
- Go 1.24+ (for building binaries)
- Node.js 22+ (for GUI frontend, optional)
