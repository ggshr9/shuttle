#!/usr/bin/env bash
# scripts/install-linux.sh
# Thin wrapper for deploy/install.sh, providing URL parity
# (scripts/install-{linux,macos,windows}.{sh,ps1}).
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec "${SCRIPT_DIR}/../deploy/install.sh" "$@"
