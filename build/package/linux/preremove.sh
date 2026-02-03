#!/bin/sh
set -e

# Stop services before removal
if command -v systemctl >/dev/null 2>&1; then
    systemctl stop shuttle 2>/dev/null || true
    systemctl stop shuttled 2>/dev/null || true
    systemctl disable shuttle 2>/dev/null || true
    systemctl disable shuttled 2>/dev/null || true
fi
