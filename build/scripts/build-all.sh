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

# Optional mobile builds. Gate behind --mobile flag / MOBILE=1 env var
# so `build-all.sh` stays fast on non-mobile hosts.
HAS_MOBILE_FLAG=0
for arg in "$@"; do
    [[ "$arg" == "--mobile" ]] && HAS_MOBILE_FLAG=1
done
if [[ "${MOBILE:-0}" == "1" || "$HAS_MOBILE_FLAG" == "1" ]]; then
    echo ""
    echo "Building mobile (--mobile)..."
    SCRIPT_DIR="$(dirname "$0")"

    # Android: attempted on any host with gomobile + Android SDK.
    if command -v gomobile >/dev/null 2>&1; then
        "$SCRIPT_DIR/build-android.sh" "$VERSION" || \
            echo "  android build failed (non-fatal, see log above)"
    else
        echo "  skipping android: gomobile not installed"
    fi

    # iOS: macOS only.
    if [[ "$(uname)" == "Darwin" ]]; then
        "$SCRIPT_DIR/build-ios.sh" "$VERSION" || \
            echo "  ios build failed (non-fatal, see log above)"
    else
        echo "  skipping ios: not macOS"
    fi
fi

ls -lh "$DIST_DIR"
