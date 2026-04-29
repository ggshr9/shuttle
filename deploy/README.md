# Shuttle Server Deployment

Two deployment options: **Docker** (recommended) or **bare-metal** via install script.

## Option A: Docker Compose

### Prerequisites

- Docker Engine 24+ and Docker Compose v2
- Ports 443 (TCP+UDP) open in your firewall
- A domain with DNS A record (optional — IP-only works too)

### Quick Start

```bash
cd deploy
cp .env.example .env
# Edit .env — see below
docker compose up -d
```

### Configuration

| Variable | Required | Description |
|---|---|---|
| `SHUTTLE_DOMAIN` | No | Domain name or IP. Auto-detects public IP if empty |
| `SHUTTLE_PASSWORD` | No | Client auth password. Auto-generated if empty |
| `SHUTTLE_EMAIL` | No | Email for Let's Encrypt (only needed with domain + Caddy) |
| `SHUTTLE_TRANSPORT` | No | `h3`, `reality`, or `both` (default: `both`) |
| `SHUTTLE_ADMIN_TOKEN` | No | Admin panel token (auto-generated if empty) |
| `SHUTTLE_DEBUG` | No | Enable debug logging (`true`/`false`) |

#### With a domain (full stack)

```bash
SHUTTLE_DOMAIN=proxy.example.com
SHUTTLE_PASSWORD=your-password
SHUTTLE_EMAIL=you@example.com
```

Runs shuttled + Caddy (auto-HTTPS admin panel on port 8443).

#### With IP only (no Caddy needed)

```bash
SHUTTLE_DOMAIN=
SHUTTLE_PASSWORD=your-password
```

Run without Caddy:

```bash
docker compose up -d shuttled
```

### Viewing Logs

```bash
docker compose logs -f shuttled   # proxy server logs
docker compose logs -f caddy      # Caddy/TLS logs
```

### Updating

```bash
docker compose pull
docker compose up -d --build
```

### Data Persistence

- `shuttle-data` volume: server config and state (`/data`)
- `caddy-data` volume: Caddy state
- `caddy-certs` volume: TLS certificates (shared read-only with shuttled)

To back up config:

```bash
docker compose cp shuttled:/data/server.yaml ./server.yaml.bak
```

---

## Option B: Install Script (Bare Metal)

### Interactive Wizard

```bash
curl -fsSL https://raw.githubusercontent.com/ggshr9/shuttle/main/deploy/install.sh -o install.sh
sudo bash install.sh
```

The wizard guides you through 3 steps:

1. **Server address** — choose domain or IP (auto-detects your public IP)
2. **Password** — set one or auto-generate
3. **Transport** — H3+Reality (default), H3 only, or Reality only

At the end it prints a QR code and import URI to share with clients.

### Non-Interactive (CI/automation)

```bash
# Minimal — auto-detects IP, auto-generates password
sudo bash install.sh install --auto

# Explicit
sudo SHUTTLE_DOMAIN=proxy.example.com \
     SHUTTLE_PASSWORD=secret \
     SHUTTLE_TRANSPORT=both \
     bash install.sh install --auto
```

### Management

```bash
sudo bash install.sh status            # Show service status + config
sudo bash install.sh upgrade           # Upgrade to latest release
sudo bash install.sh upgrade v0.2.0    # Upgrade to specific version
sudo bash install.sh uninstall         # Remove binary and service
```

```bash
systemctl status shuttled              # Service status
systemctl restart shuttled             # Restart
journalctl -u shuttled -f              # Live logs
shuttled share -c /etc/shuttle/server.yaml  # Re-print import URI + QR
```

---

## Troubleshooting

**Caddy fails to obtain certificate**
- Verify DNS A record: `dig +short your-domain.com`
- Ensure port 80 is open (ACME HTTP-01 challenge)
- Check logs: `docker compose logs caddy`

**Health check failing**
- Admin API runs on port 9090 inside the container
- Check logs: `docker compose logs shuttled`

**Port 443 already in use**
- Stop existing web server/proxy on port 443
- Or change host port mapping in `docker-compose.yml`

**Password was auto-generated and lost**
- Docker: `docker compose exec shuttled cat /data/.credentials`
- Bare metal: `cat /etc/shuttle/server.yaml | grep password`
- Or re-generate: `shuttled share -c /etc/shuttle/server.yaml`
