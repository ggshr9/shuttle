#!/bin/bash
# Build Shuttle iOS xcarchive (unsigned).
# Usage: ./build/scripts/build-ios.sh [version]
#
# Prereqs (host):
#   - macOS with Xcode 15+
#   - Go 1.24+ (for gomobile bind)
#   - gomobile: go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init
#   - Node.js 22+ (for SPA build)
#
# Produces an unsigned xcarchive. Signing / ipa export is a separate
# step (developer has to apply their provisioning profile).

set -euo pipefail

VERSION="${1:-dev}"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
DIST_DIR="${DIST_DIR:-$ROOT/dist}"

echo "Building Shuttle iOS ${VERSION}..."

if [[ "$(uname)" != "Darwin" ]]; then
    echo "error: iOS build requires macOS" >&2
    exit 1
fi

mkdir -p "$DIST_DIR"

# 1. Build the SPA.
(cd "$ROOT/gui/web" && npm ci && npm run build)

# 2. Stage SPA output inside the iOS bundle tree.
WWW_DIR="$ROOT/mobile/ios/Shuttle/www"
mkdir -p "$WWW_DIR"
rm -rf "${WWW_DIR:?}"/*
cp -R "$ROOT/gui/web/dist"/. "$WWW_DIR/"

# 3. gomobile bind → Shuttle.xcframework.
(cd "$ROOT" && gomobile bind \
    -target=ios,iossimulator \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o "$ROOT/mobile/ios/Shuttle.xcframework" \
    ./mobile)

# 4. xcodebuild archive (unsigned).
ARCHIVE_PATH="$DIST_DIR/Shuttle-${VERSION}.xcarchive"
rm -rf "$ARCHIVE_PATH"

XCPROJ="$ROOT/mobile/ios/Shuttle.xcodeproj"
if [[ ! -d "$XCPROJ" ]]; then
    echo "error: Xcode project missing at $XCPROJ" >&2
    echo "  scaffold it once in Xcode, then re-run this script" >&2
    exit 1
fi

xcodebuild archive \
    -project "$XCPROJ" \
    -scheme Shuttle \
    -configuration Release \
    -archivePath "$ARCHIVE_PATH" \
    CODE_SIGNING_ALLOWED=NO \
    CODE_SIGNING_REQUIRED=NO

echo "iOS archive: $ARCHIVE_PATH"
echo "(Run 'xcodebuild -exportArchive' with a provisioning profile to produce an ipa.)"
echo "iOS build complete."
