#!/bin/sh
set -e

# --- Defaults ---
DATA_DIR="${SHUTTLE_DATA_DIR:-/etc/shuttle}"
CONFIG="$DATA_DIR/server.yaml"

# --- Auto-generate password if missing or placeholder ---
if [ -z "$SHUTTLE_PASSWORD" ] || [ "$SHUTTLE_PASSWORD" = "change-me-to-a-strong-password" ]; then
    SHUTTLE_PASSWORD=$(head -c 32 /dev/urandom | base64 | tr -d '/+=' | head -c 24)
fi

# --- Auto-generate admin token if missing ---
if [ -z "$SHUTTLE_ADMIN_TOKEN" ]; then
    SHUTTLE_ADMIN_TOKEN=$(head -c 32 /dev/urandom | base64 | tr -d '/+=' | head -c 32)
fi

# --- Save credentials to a file (not stdout/Docker logs) ---
CREDS_FILE="${SHUTTLE_DATA_DIR:-/etc/shuttle}/.credentials"
mkdir -p "$(dirname "$CREDS_FILE")"
echo "Password: $SHUTTLE_PASSWORD" > "$CREDS_FILE"
echo "Admin Token: $SHUTTLE_ADMIN_TOKEN" >> "$CREDS_FILE"
chmod 600 "$CREDS_FILE"
echo "==> Credentials saved to $CREDS_FILE"

# --- Ensure data directory exists ---
mkdir -p "$DATA_DIR"

# --- Generate default server config if not present ---
if [ ! -f "$CONFIG" ]; then
    echo "No config found at $CONFIG. Running auto-init..."
    INIT_ARGS="--dir $DATA_DIR --password $SHUTTLE_PASSWORD"
    INIT_ARGS="$INIT_ARGS --transport ${SHUTTLE_TRANSPORT:-both}"
    if [ -n "${SHUTTLE_DOMAIN:-}" ]; then
        INIT_ARGS="$INIT_ARGS --domain $SHUTTLE_DOMAIN"
    fi
    shuttled init $INIT_ARGS
    echo ""
fi

# --- Wait for TLS certs if /certs is mounted ---
if [ -d /certs ]; then
    echo "Waiting for TLS certificates..."
    ATTEMPTS=0
    while [ $ATTEMPTS -lt 30 ]; do
        # Caddy stores certs under /certs/acme-v02.api.letsencrypt.org-directory/
        if find /certs -name "*.crt" -o -name "*.pem" 2>/dev/null | head -1 | grep -q .; then
            echo "TLS certificates found."
            break
        fi
        ATTEMPTS=$((ATTEMPTS + 1))
        sleep 2
    done
    if [ $ATTEMPTS -eq 30 ]; then
        echo "Warning: No TLS certs found after 60s — continuing without external certs."
    fi
fi

# --- Print connection info ---
echo ""
echo "========================================"
echo "  Shuttle Server Starting"
echo "----------------------------------------"
if [ -n "${SHUTTLE_DOMAIN:-}" ]; then
    echo "  Domain:    $SHUTTLE_DOMAIN"
    echo "  Admin:     https://$SHUTTLE_DOMAIN"
else
    echo "  Mode:      IP-only"
    echo "  Listen:    :443"
fi
echo "  Transport: ${SHUTTLE_TRANSPORT:-both}"
echo "  Config:    $CONFIG"
echo "========================================"
echo ""

exec shuttled run -c "$CONFIG"
