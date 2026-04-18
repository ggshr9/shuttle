#!/bin/bash
# Shuttle — one-line install and run
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/ggshr9/shuttle/main/scripts/install.sh | bash
#   sudo shuttled install -c <config> --ui :9090    # server (install systemd service)
#   shuttle install -c config.yaml                  # client (install user service)
#   <binary> status | stop | restart | logs -f | uninstall  # manage

set -e

REPO="ggshr9/shuttle"
VERSION="v0.3.4"
INSTALL_DIR="/usr/local/bin"

# Detect OS and arch
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

echo "Installing Shuttle $VERSION ($OS/$ARCH)..."

# Download both client and server
for BIN in shuttle shuttled; do
    URL="https://github.com/$REPO/releases/download/$VERSION/${BIN}-${OS}-${ARCH}.gz"
    echo "  Downloading $BIN..."
    curl -fsSL "$URL" | gunzip > /tmp/$BIN
    chmod +x /tmp/$BIN
    if [ -w "$INSTALL_DIR" ]; then
        mv /tmp/$BIN "$INSTALL_DIR/$BIN"
    else
        sudo mv /tmp/$BIN "$INSTALL_DIR/$BIN"
    fi
done

echo ""
echo "Installed: $(shuttle version), $(shuttled version)"
echo ""
echo "Quick start:"
echo "  Server:"
echo "    sudo shuttled install -c <config> --ui :9090"
echo "    shuttled run -p yourpassword                  (foreground)"
echo ""
echo "  Client:"
echo "    shuttle run -u \"shuttle://...\""
echo "    shuttle install -c config.yaml                (run as user service)"
echo ""
echo "  Manage: <binary> status | stop | restart | logs -f | token -c <config> | uninstall"
echo ""
echo "That's it."
