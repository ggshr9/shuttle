#!/bin/bash
# Build Shuttle Android APK.
# Usage: ./build/scripts/build-android.sh [version]
#
# Prereqs (host):
#   - Go 1.24+ (for gomobile bind)
#   - gomobile: go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init
#   - Node.js 22+ (for SPA build)
#   - Android SDK + Gradle (gradlew) in mobile/android/
#   - ANDROID_HOME env var set

set -euo pipefail

VERSION="${1:-dev}"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
DIST_DIR="${DIST_DIR:-$ROOT/dist}"

echo "Building Shuttle Android ${VERSION}..."

mkdir -p "$DIST_DIR"

# 1. Build the SPA (Svelte → static files).
(cd "$ROOT/gui/web" && npm ci && npm run build)

# 2. Stage SPA output inside the Android assets tree.
ASSETS_WEB="$ROOT/mobile/android/app/src/main/assets/web"
mkdir -p "$ASSETS_WEB"
rm -rf "${ASSETS_WEB:?}"/*
cp -R "$ROOT/gui/web/dist"/. "$ASSETS_WEB/"

# 3. gomobile bind → shuttle.aar.
LIBS_DIR="$ROOT/mobile/android/app/libs"
mkdir -p "$LIBS_DIR"
#    -checklinkname=0 — required by github.com/wlynxg/anet (transitive
#    via pion). anet uses //go:linkname to access net.zoneCache; Go 1.23+
#    enforces strict linkname rules unless this flag opts out. See anet
#    README. Drop once anet ships a fix or a replacement is found.
(cd "$ROOT" && gomobile bind \
    -target=android \
    -androidapi=24 \
    -ldflags="-s -w -checklinkname=0 -X main.version=${VERSION}" \
    -o "$LIBS_DIR/shuttle.aar" \
    ./mobile)

# 4. Gradle assembleRelease.
if [[ ! -x "$ROOT/mobile/android/gradlew" ]]; then
    echo "error: gradle wrapper missing at mobile/android/gradlew" >&2
    echo "  run 'gradle wrapper --gradle-version 8.5' once from mobile/android/" >&2
    exit 1
fi
(cd "$ROOT/mobile/android" && ./gradlew assembleRelease)

# 5. Copy APK + AAB into DIST_DIR.
APK_SRC="$ROOT/mobile/android/app/build/outputs/apk/release/app-release-unsigned.apk"
AAB_SRC="$ROOT/mobile/android/app/build/outputs/bundle/release/app-release.aab"
APK_OUT="$DIST_DIR/shuttle-android-${VERSION}.apk"
AAB_OUT="$DIST_DIR/shuttle-android-${VERSION}.aab"

if [[ -f "$APK_SRC" ]]; then
    cp "$APK_SRC" "$APK_OUT"
    echo "APK: $APK_OUT"
fi
if [[ -f "$AAB_SRC" ]]; then
    cp "$AAB_SRC" "$AAB_OUT"
    echo "AAB: $AAB_OUT"
fi

echo "Android build complete."
