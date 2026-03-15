#!/bin/bash
#
# Cross-compile shuttle and shuttled for all supported platforms.
#
# Usage:
#   ./scripts/build-all.sh              # Build all platforms
#   VERSION=v1.2.3 ./scripts/build-all.sh  # Build with specific version
#
# Output: dist/ directory with archives per platform

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

VERSION=${VERSION:-$(git describe --tags --always 2>/dev/null || echo "dev")}
LDFLAGS="-s -w -X main.version=${VERSION}"
DIST_DIR="${PROJECT_DIR}/dist"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

log() { echo -e "${BLUE}[build]${NC} $1"; }
success() { echo -e "${GREEN}[done]${NC} $1"; }

PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
    "linux/arm"
    "linux/mipsle"
    "linux/mips"
    "darwin/amd64"
    "darwin/arm64"
    "windows/amd64"
    "freebsd/amd64"
)

rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

log "Building Shuttle ${VERSION} for ${#PLATFORMS[@]} platforms..."
echo ""

for platform in "${PLATFORMS[@]}"; do
    GOOS="${platform%/*}"
    GOARCH="${platform#*/}"
    EXT=""
    ARCHIVE_EXT="tar.gz"

    if [ "$GOOS" = "windows" ]; then
        EXT=".exe"
        ARCHIVE_EXT="zip"
    fi

    GOMIPS_FLAG=""
    if [ "$GOARCH" = "mipsle" ] || [ "$GOARCH" = "mips" ]; then
        GOMIPS_FLAG="softfloat"
    fi

    OUTPUT_DIR="${DIST_DIR}/shuttle-${GOOS}-${GOARCH}"
    mkdir -p "$OUTPUT_DIR"

    log "Building ${GOOS}/${GOARCH}..."

    CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" GOMIPS="$GOMIPS_FLAG" \
        go build -ldflags="$LDFLAGS" -o "${OUTPUT_DIR}/shuttle${EXT}" ./cmd/shuttle

    CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" GOMIPS="$GOMIPS_FLAG" \
        go build -ldflags="$LDFLAGS" -o "${OUTPUT_DIR}/shuttled${EXT}" ./cmd/shuttled

    # Create archive
    cd "$DIST_DIR"
    if [ "$ARCHIVE_EXT" = "zip" ]; then
        zip -qr "shuttle-${GOOS}-${GOARCH}.zip" "shuttle-${GOOS}-${GOARCH}/"
    else
        tar czf "shuttle-${GOOS}-${GOARCH}.tar.gz" "shuttle-${GOOS}-${GOARCH}/"
    fi
    rm -rf "$OUTPUT_DIR"
    cd "$PROJECT_DIR"

    success "${GOOS}/${GOARCH}"
done

echo ""

# Generate checksums
cd "$DIST_DIR"
if command -v sha256sum &>/dev/null; then
    sha256sum * > checksums.txt
elif command -v shasum &>/dev/null; then
    shasum -a 256 * > checksums.txt
fi

log "Checksums:"
cat checksums.txt
echo ""

success "All builds complete. Output: ${DIST_DIR}/"
ls -lh "$DIST_DIR"
