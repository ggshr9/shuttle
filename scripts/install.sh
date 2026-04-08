#!/bin/bash
# Shuttle — one-line install and run
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/ggshr9/shuttle/main/scripts/install.sh | bash
#   shuttled run -p yourpassword                    # server
#   shuttle import "shuttle://..." && shuttle run    # client

set -e

REPO="ggshr9/shuttle"
VERSION="v0.3.2"
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
echo "  Server:  shuttled run -p yourpassword"
echo "  Client:  shuttle import \"shuttle://...\" && shuttle run"
echo ""
echo "That's it."
