#!/bin/sh
set -e

# Create config directory if not exists
mkdir -p /etc/shuttle

# Reload systemd
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload
fi

echo "Shuttle installed successfully."
echo "  - Edit /etc/shuttle/client.yaml for client configuration"
echo "  - Edit /etc/shuttle/server.yaml for server configuration"
echo "  - Start client: systemctl start shuttle"
echo "  - Start server: systemctl start shuttled"
