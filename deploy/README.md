# Shuttle Docker Deployment

One-click deployment with auto-HTTPS via Caddy.

## Prerequisites

- Docker Engine 24+ and Docker Compose v2
- A domain with a DNS A record pointing to your server's public IP
- Ports 80, 443 (TCP+UDP), and 8443 open in your firewall

## Quick Start

```bash
cd deploy
cp .env.example .env
# Edit .env — set SHUTTLE_DOMAIN, SHUTTLE_PASSWORD, SHUTTLE_EMAIL
docker compose up -d
```

The stack starts two services:

- **shuttled** — proxy server on port 443 (H3/QUIC over UDP, Reality/TLS over TCP)
- **caddy** — auto-HTTPS reverse proxy for the admin panel (port 80/8443)

## Configuration Reference

| Variable | Required | Description |
|---|---|---|
| `SHUTTLE_DOMAIN` | Yes | Domain name (must have DNS A record) |
| `SHUTTLE_PASSWORD` | Yes | Client authentication password |
| `SHUTTLE_EMAIL` | Yes | Email for Let's Encrypt certificates |
| `SHUTTLE_ADMIN_TOKEN` | No | Admin panel token (auto-generated if empty) |
| `SHUTTLE_DEBUG` | No | Enable debug logging (`true`/`false`) |

If `SHUTTLE_PASSWORD` is left empty or set to the placeholder value, a random password is generated on first start and printed to the logs.

## Viewing Logs

```bash
docker compose logs -f shuttled   # proxy server logs
docker compose logs -f caddy      # Caddy/TLS logs
```

## Updating

```bash
docker compose pull
docker compose up -d --build
```

## Data Persistence

- `shuttle-data` volume: server config and state (`/data`)
- `caddy-data` volume: Caddy state
- `caddy-certs` volume: TLS certificates (shared read-only with shuttled)

To back up your config:

```bash
docker compose cp shuttled:/data/server.yaml ./server.yaml.bak
```

## Simple Mode (no Caddy)

If you manage TLS externally or don't need the admin panel, run shuttled alone:

```bash
docker compose up -d shuttled
```

## Troubleshooting

**Caddy fails to obtain certificate**
- Verify your DNS A record resolves to this server: `dig +short your-domain.com`
- Ensure port 80 is open (needed for ACME HTTP-01 challenge)
- Check Caddy logs: `docker compose logs caddy`

**Health check failing**
- The admin API runs on port 9090 inside the container
- Check shuttled logs: `docker compose logs shuttled`

**Port 443 already in use**
- Stop any existing web server or proxy on port 443
- Or change the host port mapping in `docker-compose.yml`

**Password was auto-generated and lost**
- Check the initial startup logs: `docker compose logs shuttled | head -20`
- Or edit the config directly: `docker compose exec shuttled cat /data/server.yaml`
