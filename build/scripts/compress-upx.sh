#!/bin/bash
# Compress binaries with UPX for embedded systems
# Requires: upx (https://upx.github.io/)
# Usage: ./build/scripts/compress-upx.sh

set -e

DIST_DIR="${1:-dist}"

if ! command -v upx &> /dev/null; then
    echo "Error: UPX not found. Install from https://upx.github.io/"
    exit 1
fi

echo "Compressing binaries with UPX..."

# Only compress embedded targets (MIPS, ARM)
for binary in "$DIST_DIR"/*-mips* "$DIST_DIR"/*-arm*; do
    [[ -f "$binary" ]] || continue
    [[ "$binary" == *.upx ]] && continue

    echo "  -> $(basename "$binary")"
    upx --best -q -o "${binary}.upx" "$binary"
done

echo ""
echo "Compressed binaries:"
ls -lh "$DIST_DIR"/*.upx 2>/dev/null || echo "No compressed binaries found"
