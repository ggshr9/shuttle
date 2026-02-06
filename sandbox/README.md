# Shuttle Sandbox Test Environment

Docker-based isolated testing environment for Shuttle.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Docker Compose Sandbox                    │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│   ┌─────────────┐                      ┌─────────────┐      │
│   │  client-a   │                      │  client-b   │      │
│   │  (shuttle)  │                      │  (shuttle)  │      │
│   │ 10.100.1.10 │                      │ 10.100.2.10 │      │
│   └──────┬──────┘                      └──────┬──────┘      │
│          │                                    │              │
│          │  net-a                    net-b    │              │
│          │  10.100.1.0/24      10.100.2.0/24  │              │
│          │                                    │              │
│   ┌──────┴────────────────────────────────────┴──────┐      │
│   │                     router                        │      │
│   │              (NAT + Firewall)                     │      │
│   │                   10.100.0.1                      │      │
│   └──────────────────────┬───────────────────────────┘      │
│                          │                                   │
│                   net-server                                 │
│                   10.100.0.0/24                              │
│                          │                                   │
│               ┌──────────┴──────────┐                       │
│               │      server         │                       │
│               │    (shuttled)       │                       │
│               │    10.100.0.10      │                       │
│               └─────────────────────┘                       │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

## Quick Start

```bash
# Run all tests
./run.sh

# Or step by step:
./run.sh build   # Build binaries and images
./run.sh up      # Start environment
./run.sh test    # Run tests
./run.sh down    # Stop environment
```

## Commands

| Command | Description |
|---------|-------------|
| `./run.sh` | Run everything (build, start, test, stop) |
| `./run.sh build` | Build binaries and Docker images |
| `./run.sh up` | Start the sandbox environment |
| `./run.sh down` | Stop the sandbox environment |
| `./run.sh test` | Run test suite |
| `./run.sh logs` | View container logs |
| `./run.sh logs server` | View specific container logs |
| `./run.sh shell server` | Open shell in container |
| `./run.sh clean` | Clean up everything |

## Test Cases

| Test | Description |
|------|-------------|
| `test_server_api` | Server API is responding |
| `test_client_a_api` | Client A API is responding |
| `test_client_b_api` | Client B API is responding |
| `test_client_a_to_router` | Client A can reach router |
| `test_client_b_to_router` | Client B can reach router |
| `test_socks5_proxy` | SOCKS5 proxy works |
| `test_http_proxy` | HTTP proxy works |

## Network Simulation

The router container can simulate various network conditions:

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

## Debugging

```bash
# View all logs
./run.sh logs

# View specific service logs
./run.sh logs server
./run.sh logs client-a

# Shell into container
./run.sh shell server
./run.sh shell client-a

# Check container status
docker compose ps

# Check network connectivity
docker compose exec client-a ping 10.100.0.1
docker compose exec client-a curl http://10.100.0.10:9090/api/status
```

## Requirements

- Docker
- Docker Compose v2
- Go 1.24+ (for building binaries)
