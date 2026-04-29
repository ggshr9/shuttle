#!/usr/bin/env bash
set -euo pipefail

REPO="ggshr9/shuttle"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/shuttle"
SERVICE_FILE="/etc/systemd/system/shuttled.service"

# ── Colors ──
BOLD='\033[1m'
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
DIM='\033[2m'
NC='\033[0m'

info()  { echo -e "${GREEN}▸${NC} $*"; }
warn()  { echo -e "${YELLOW}▸${NC} $*"; }
error() { echo -e "${RED}✗${NC} $*"; exit 1; }

banner() {
    echo ""
    echo -e "${CYAN}╔══════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║${NC}   ${BOLD}Shuttle Server — Setup Wizard${NC}          ${CYAN}║${NC}"
    echo -e "${CYAN}╚══════════════════════════════════════════╝${NC}"
    echo ""
}

# ── Platform detection ──

detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64|amd64) ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        armv7*|armv6*) ARCH="arm" ;;
        mips*) ARCH="mipsle" ;;
        *) error "Unsupported architecture: $ARCH" ;;
    esac
    case "$OS" in
        linux) ;;
        *) error "This script supports Linux only. Got: $OS" ;;
    esac
    info "Platform: ${OS}/${ARCH}"
}

# ── Download binary ──

download_binary() {
    local version="${1:-latest}"
    local url

    if [ "$version" = "latest" ]; then
        url="https://github.com/${REPO}/releases/latest/download/shuttled-${OS}-${ARCH}"
    else
        url="https://github.com/${REPO}/releases/download/${version}/shuttled-${OS}-${ARCH}"
    fi

    info "Downloading shuttled..."
    echo -e "  ${DIM}${url}${NC}"
    curl -fsSL -o "${INSTALL_DIR}/shuttled" "$url" || error "Download failed. Check your network or the release URL."
    chmod +x "${INSTALL_DIR}/shuttled"
    info "Installed to ${INSTALL_DIR}/shuttled"
}

# ── Auto-detect public IP ──

detect_public_ip() {
    local ip=""
    # Try multiple services in case one is down
    for svc in "https://api.ipify.org" "https://ifconfig.me" "https://icanhazip.com"; do
        ip=$(curl -fsSL --connect-timeout 5 "$svc" 2>/dev/null | tr -d '[:space:]') && break
    done
    echo "$ip"
}

# ── Interactive setup wizard ──

wizard() {
    banner

    local domain=""
    local password=""
    local transport=""
    local addr_mode=""

    # ── Step 1: Domain or IP ──
    echo -e "${BOLD}Step 1/3 — Server Address${NC}"
    echo ""
    echo "  1) Use a domain name  (e.g. proxy.example.com)"
    echo -e "     ${DIM}Recommended if you have a domain. Enables all transports.${NC}"
    echo ""
    echo "  2) Use server IP address"
    echo -e "     ${DIM}No domain needed. Works with H3/QUIC and Reality transports.${NC}"
    echo ""

    while true; do
        read -rp "  Choose [1/2] (default: 2): " addr_mode
        addr_mode="${addr_mode:-2}"
        case "$addr_mode" in
            1)
                echo ""
                read -rp "  Enter your domain: " domain
                [ -z "$domain" ] && { warn "Domain cannot be empty."; continue; }
                echo ""
                info "Using domain: ${BOLD}${domain}${NC}"
                break
                ;;
            2)
                echo ""
                echo -e "  ${DIM}Detecting public IP...${NC}"
                local detected_ip
                detected_ip=$(detect_public_ip)
                if [ -n "$detected_ip" ]; then
                    echo ""
                    read -rp "  Server IP [${detected_ip}]: " user_ip
                    domain="${user_ip:-$detected_ip}"
                else
                    echo ""
                    read -rp "  Could not detect IP. Enter server IP: " domain
                    [ -z "$domain" ] && { warn "IP cannot be empty."; continue; }
                fi
                echo ""
                info "Using IP: ${BOLD}${domain}${NC}"
                break
                ;;
            *) warn "Please enter 1 or 2." ;;
        esac
    done

    # ── Step 2: Password ──
    echo ""
    echo -e "${BOLD}Step 2/3 — Authentication${NC}"
    echo ""
    read -rp "  Set a password (leave empty to auto-generate): " password
    if [ -z "$password" ]; then
        password=$(head -c 32 /dev/urandom | base64 | tr -d '/+=' | head -c 16)
        echo ""
        info "Generated password: ${BOLD}${password}${NC}"
    fi

    # ── Step 3: Transport ──
    echo ""
    echo -e "${BOLD}Step 3/3 — Transport Protocol${NC}"
    echo ""
    echo "  1) Both H3 + Reality  ${DIM}(recommended)${NC}"
    echo "  2) H3/QUIC only      ${DIM}(fast, UDP-based)${NC}"
    echo "  3) Reality only       ${DIM}(TLS camouflage, TCP-based)${NC}"
    echo ""

    while true; do
        read -rp "  Choose [1/2/3] (default: 1): " transport_choice
        transport_choice="${transport_choice:-1}"
        case "$transport_choice" in
            1) transport="both"; break ;;
            2) transport="h3"; break ;;
            3) transport="reality"; break ;;
            *) warn "Please enter 1, 2, or 3." ;;
        esac
    done

    echo ""
    echo -e "${CYAN}────────────────────────────────────────────${NC}"
    echo ""

    # ── Run shuttled init ──
    local init_args="--dir ${CONFIG_DIR} --password ${password} --transport ${transport}"
    if [ -n "$domain" ]; then
        init_args="${init_args} --domain ${domain}"
    fi

    info "Generating config..."
    ${INSTALL_DIR}/shuttled init ${init_args}
}

# ── Non-interactive setup (env vars or flags) ──

auto_configure() {
    local domain="${SHUTTLE_DOMAIN:-}"
    local password="${SHUTTLE_PASSWORD:-}"
    local transport="${SHUTTLE_TRANSPORT:-both}"
    local args="--dir ${CONFIG_DIR} --transport ${transport}"

    if [ -n "$domain" ]; then
        args="$args --domain $domain"
    fi
    if [ -n "$password" ]; then
        args="$args --password $password"
    fi

    info "Running auto-config..."
    ${INSTALL_DIR}/shuttled init $args
}

# ── Create system user ──

create_user() {
    if ! id -u shuttle &>/dev/null; then
        useradd -r -s /sbin/nologin -d /nonexistent shuttle
        info "Created system user: shuttle"
    fi
    chown -R shuttle:shuttle "$CONFIG_DIR"
}

# ── Install systemd service ──

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

# ── Uninstall ──

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

# ── Status ──

status() {
    if systemctl is-active --quiet shuttled 2>/dev/null; then
        info "shuttled is ${GREEN}running${NC}"
        systemctl status shuttled --no-pager -l 2>/dev/null | head -15
    else
        warn "shuttled is not running"
        echo ""
        echo "  Start:   systemctl start shuttled"
        echo "  Logs:    journalctl -u shuttled -f"
    fi
    if [ -f "${CONFIG_DIR}/server.yaml" ]; then
        echo ""
        info "Config: ${CONFIG_DIR}/server.yaml"
        echo -e "  ${DIM}View import URI: shuttled share -c ${CONFIG_DIR}/server.yaml${NC}"
    fi
}

# ── Usage ──

usage() {
    echo ""
    echo -e "${BOLD}Shuttle Server Installer${NC}"
    echo ""
    echo "Usage: $0 <command> [options]"
    echo ""
    echo "Commands:"
    echo "  install          Interactive setup wizard (default)"
    echo "  install --auto   Non-interactive setup (uses env vars)"
    echo "  uninstall        Remove shuttled binary and service"
    echo "  upgrade [ver]    Upgrade binary (e.g. upgrade v0.2.0)"
    echo "  status           Show service status and config info"
    echo ""
    echo "Environment variables (for --auto mode):"
    echo "  SHUTTLE_DOMAIN      Domain or IP (auto-detects IP if empty)"
    echo "  SHUTTLE_PASSWORD    Auth password (auto-generated if empty)"
    echo "  SHUTTLE_TRANSPORT   h3, reality, or both (default: both)"
    echo ""
    echo "Examples:"
    echo "  sudo bash install.sh                           # Interactive wizard"
    echo "  sudo SHUTTLE_PASSWORD=secret bash install.sh install --auto"
    echo "  sudo bash install.sh upgrade v0.2.0"
    echo ""
}

# ── Main ──

main() {
    local action="${1:-install}"
    shift || true

    # Allow --help anywhere
    for arg in "$@" "$action"; do
        case "$arg" in
            -h|--help|help) usage; exit 0 ;;
        esac
    done

    # Root check (except for help/status)
    case "$action" in
        status) status; exit 0 ;;
    esac
    [ "$(id -u)" -ne 0 ] && error "Please run as root (use sudo)"

    case "$action" in
        install)
            detect_platform
            download_binary "${1:-latest}"
            if [ "${1:-}" = "--auto" ]; then
                auto_configure
            else
                wizard
            fi
            create_user
            install_service
            echo ""
            echo -e "${GREEN}╔══════════════════════════════════════════╗${NC}"
            echo -e "${GREEN}║${NC}   ${BOLD}Setup complete! shuttled is running.${NC}   ${GREEN}║${NC}"
            echo -e "${GREEN}╚══════════════════════════════════════════╝${NC}"
            echo ""
            echo "  Manage:   systemctl {start|stop|restart} shuttled"
            echo "  Logs:     journalctl -u shuttled -f"
            echo "  Status:   $0 status"
            echo "  Share:    shuttled share -c ${CONFIG_DIR}/server.yaml"
            echo ""
            ;;
        uninstall)
            uninstall
            ;;
        upgrade)
            detect_platform
            systemctl stop shuttled 2>/dev/null || true
            download_binary "${1:-latest}"
            systemctl start shuttled
            info "Upgrade complete. shuttled restarted."
            ;;
        *)
            usage
            exit 1
            ;;
    esac
}

main "$@"
