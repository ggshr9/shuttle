#!/bin/bash
# Build script for all platforms
# Usage: ./build/scripts/build-all.sh [version]

set -e

VERSION="${1:-dev}"
DIST_DIR="dist"
LDFLAGS="-s -w -X main.version=${VERSION}"

mkdir -p "$DIST_DIR"

echo "Building Shuttle ${VERSION}..."

# Build matrix
TARGETS=(
    "linux/amd64"
    "linux/arm64"
    "linux/arm"
    "linux/mipsle/softfloat"   # OpenWrt MIPS
    "linux/mips/softfloat"     # OpenWrt MIPS big-endian
    "darwin/amd64"
    "darwin/arm64"
    "windows/amd64"
    "freebsd/amd64"
)

for target in "${TARGETS[@]}"; do
    IFS='/' read -r goos goarch gomips <<< "$target"

    ext=""
    [[ "$goos" == "windows" ]] && ext=".exe"

    output_client="${DIST_DIR}/shuttle-${goos}-${goarch}${ext}"
    output_server="${DIST_DIR}/shuttled-${goos}-${goarch}${ext}"

    echo "  -> ${goos}/${goarch}${gomips:+ (GOMIPS=$gomips)}"

    env CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" GOMIPS="$gomips" \
        go build -ldflags="$LDFLAGS" -o "$output_client" ./cmd/shuttle

    env CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" GOMIPS="$gomips" \
        go build -ldflags="$LDFLAGS" -o "$output_server" ./cmd/shuttled
done

echo ""
echo "Build complete. Binaries in ${DIST_DIR}/"
ls -lh "$DIST_DIR"
