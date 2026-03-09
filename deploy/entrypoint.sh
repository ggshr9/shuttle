#!/bin/sh
set -e

CONFIG=/etc/shuttle/server.yaml

# Auto-init if no config exists
if [ ! -f "$CONFIG" ]; then
    echo "No config found. Running auto-init..."
    shuttled init --dir /etc/shuttle
    echo ""
fi

exec shuttled run -c "$CONFIG"
