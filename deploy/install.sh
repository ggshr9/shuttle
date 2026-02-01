#!/usr/bin/env bash
set -euo pipefail

REPO="ggshr9/shuttle"
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

# Interactive config
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
        certbot certonly --standalone -d "$DOMAIN" --non-interactive --agree-tos --register-unsafely-without-email || true
        CERT_FILE="/etc/letsencrypt/live/${DOMAIN}/fullchain.pem"
        KEY_FILE="/etc/letsencrypt/live/${DOMAIN}/privkey.pem"
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

    info "Config written to ${CONFIG_DIR}/server.yaml"
}

# Install systemd service
install_service() {
    cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=Shuttle Server
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/shuttled run -c ${CONFIG_DIR}/server.yaml
Restart=on-failure
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable shuttled
    systemctl start shuttled
    info "shuttled service started"
}

# Print client config
print_client_config() {
    echo ""
    echo "=============================="
    echo " Client Configuration"
    echo "=============================="
    cat <<EOF

server:
  addr: "${DOMAIN}:443"
  password: "${PASSWORD}"
  sni: "${DOMAIN}"

transport:
  h3:
    enabled: true
  reality:
    enabled: true
    server_name: "www.microsoft.com"
    public_key: "${PUBLIC_KEY}"
    short_id: "0123456789abcdef"

proxy:
  socks5:
    enabled: true
    listen: "127.0.0.1:1080"
  http:
    enabled: true
    listen: "127.0.0.1:8080"

routing:
  default: "proxy"
  rules:
    - domains: "geosite:cn"
      action: "direct"
    - geoip: "cn"
      action: "direct"
EOF
    echo ""
    echo "=============================="
}

# Main
main() {
    [ "$(id -u)" -ne 0 ] && error "Please run as root"

    detect_platform
    download_binary "${1:-latest}"
    configure
    install_service
    print_client_config

    info "Setup complete! shuttled is running on ${DOMAIN}:443"
}

main "$@"
