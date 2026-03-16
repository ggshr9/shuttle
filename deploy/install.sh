#!/usr/bin/env bash
set -euo pipefail

REPO="shuttleX/shuttle"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/shuttle"
SERVICE_FILE="/etc/systemd/system/shuttled.service"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*"; exit 1; }

# Detect OS and arch
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64|amd64) ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *) error "Unsupported architecture: $ARCH" ;;
    esac
    case "$OS" in
        linux) ;;
        *) error "This script supports Linux only. Got: $OS" ;;
    esac
    info "Platform: ${OS}/${ARCH}"
}

# Download binary
download_binary() {
    local version="${1:-latest}"
    local url

    if [ "$version" = "latest" ]; then
        url="https://github.com/${REPO}/releases/latest/download/shuttled-${OS}-${ARCH}"
    else
        url="https://github.com/${REPO}/releases/download/${version}/shuttled-${OS}-${ARCH}"
    fi

    info "Downloading shuttled from ${url}..."
    curl -fsSL -o "${INSTALL_DIR}/shuttled" "$url" || error "Download failed. Check your network or the release URL."
    chmod +x "${INSTALL_DIR}/shuttled"
    info "Installed to ${INSTALL_DIR}/shuttled"
}

# Quick auto-init (zero interaction)
auto_configure() {
    local domain="${SHUTTLE_DOMAIN:-}"
    local password="${SHUTTLE_PASSWORD:-}"
    local args="--dir ${CONFIG_DIR}"

    if [ -n "$domain" ]; then
        args="$args --domain $domain"
    fi
    if [ -n "$password" ]; then
        args="$args --password $password"
    fi

    info "Running zero-config setup..."
    ${INSTALL_DIR}/shuttled init $args
}

# Interactive config (advanced)
configure() {
    echo ""
    read -rp "Server domain (e.g. example.com): " DOMAIN
    read -rp "Password: " PASSWORD
    echo ""
    read -rp "Transport [h3/reality/both] (default: both): " TRANSPORT
    TRANSPORT=${TRANSPORT:-both}

    info "Generating key pair..."
    KEYS=$("${INSTALL_DIR}/shuttled" genkey 2>&1)
    PRIVATE_KEY=$(echo "$KEYS" | grep "Private Key:" | awk -F': ' '{print $2}')
    PUBLIC_KEY=$(echo "$KEYS" | grep "Public Key:" | awk -F': ' '{print $2}')

    mkdir -p "$CONFIG_DIR"

    # TLS certificate setup
    info "Setting up TLS certificates..."
    if command -v certbot &>/dev/null; then
        # Check if port 80 is available for certbot standalone mode
        if ss -tlnp 2>/dev/null | grep -q ':80 '; then
            warn "Port 80 is in use. certbot --standalone requires port 80 to be free."
            warn "Stop the service using port 80, or use certbot with --webroot or DNS challenge."
            CERT_FILE="${CONFIG_DIR}/cert.pem"
            KEY_FILE="${CONFIG_DIR}/key.pem"
            warn "Please place your certificates at ${CERT_FILE} and ${KEY_FILE}"
        else
            certbot certonly --standalone -d "$DOMAIN" --non-interactive --agree-tos --register-unsafely-without-email || {
                warn "certbot failed. Please obtain certificates manually."
                CERT_FILE="${CONFIG_DIR}/cert.pem"
                KEY_FILE="${CONFIG_DIR}/key.pem"
            }
            CERT_FILE="${CERT_FILE:-/etc/letsencrypt/live/${DOMAIN}/fullchain.pem}"
            KEY_FILE="${KEY_FILE:-/etc/letsencrypt/live/${DOMAIN}/privkey.pem}"
        fi
    else
        warn "certbot not found. Install it or provide certificates manually."
        CERT_FILE="${CONFIG_DIR}/cert.pem"
        KEY_FILE="${CONFIG_DIR}/key.pem"
    fi

    # Build transport config
    local h3_enabled="false"
    local reality_enabled="false"
    case "$TRANSPORT" in
        h3) h3_enabled="true" ;;
        reality) reality_enabled="true" ;;
        both) h3_enabled="true"; reality_enabled="true" ;;
    esac

    cat > "${CONFIG_DIR}/server.yaml" <<EOF
listen: ":443"

tls:
  cert_file: "${CERT_FILE}"
  key_file: "${KEY_FILE}"

auth:
  password: "${PASSWORD}"
  private_key: "${PRIVATE_KEY}"
  public_key: "${PUBLIC_KEY}"

cover:
  mode: "default"

transport:
  h3:
    enabled: ${h3_enabled}
    path_prefix: "/h3"
  reality:
    enabled: ${reality_enabled}
    target_sni: "www.microsoft.com"
    target_addr: "www.microsoft.com:443"
    short_ids:
      - "0123456789abcdef"

log:
  level: "info"
EOF

    # Secure config file — contains password and private key
    chmod 600 "${CONFIG_DIR}/server.yaml"
    info "Config written to ${CONFIG_DIR}/server.yaml (permissions: 600)"
}

# Create system user
create_user() {
    if ! id -u shuttle &>/dev/null; then
        useradd -r -s /sbin/nologin -d /nonexistent shuttle
        info "Created system user: shuttle"
    fi
    chown -R shuttle:shuttle "$CONFIG_DIR"
}

# Install systemd service
install_service() {
    cat > "$SERVICE_FILE" <<'UNIT'
[Unit]
Description=Shuttle Server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=shuttle
Group=shuttle
ExecStart=/usr/local/bin/shuttled run -c /etc/shuttle/server.yaml
Restart=on-failure
RestartSec=5
StartLimitIntervalSec=300
StartLimitBurst=5
LimitNOFILE=65535

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/etc/shuttle
PrivateTmp=true
ProtectKernelTunables=true
ProtectControlGroups=true

# Allow binding to privileged ports
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
UNIT

    systemctl daemon-reload
    systemctl enable shuttled
    systemctl start shuttled
    info "shuttled service started"
}

# Uninstall
uninstall() {
    info "Uninstalling shuttled..."
    systemctl stop shuttled 2>/dev/null || true
    systemctl disable shuttled 2>/dev/null || true
    rm -f "$SERVICE_FILE"
    systemctl daemon-reload
    rm -f "${INSTALL_DIR}/shuttled"
    info "Binary and service removed."
    echo ""
    warn "Config directory ${CONFIG_DIR} was NOT removed (contains keys)."
    warn "To remove it: rm -rf ${CONFIG_DIR}"
    warn "To remove the user: userdel shuttle"
}

# Main
main() {
    [ "$(id -u)" -ne 0 ] && error "Please run as root"

    # Support subcommands
    local action="${1:-install}"
    shift || true

    case "$action" in
        install)
            detect_platform
            download_binary "${1:-latest}"
            auto_configure
            create_user
            install_service
            info "Setup complete! shuttled is running."
            info "Check the output above for the import URI and QR code."
            ;;
        install-advanced)
            detect_platform
            download_binary "${1:-latest}"
            configure
            create_user
            install_service
            info "Setup complete!"
            ;;
        uninstall)
            uninstall
            ;;
        upgrade)
            detect_platform
            systemctl stop shuttled 2>/dev/null || true
            download_binary "${1:-latest}"
            systemctl start shuttled
            info "Upgrade complete."
            ;;
        *)
            echo "Usage: $0 [install|install-advanced|uninstall|upgrade] [version]"
            echo ""
            echo "  install          Quick zero-config setup (default)"
            echo "  install-advanced Interactive setup with certbot"
            echo "  uninstall        Remove shuttled"
            echo "  upgrade          Upgrade binary"
            exit 1
            ;;
    esac
}

main "$@"
