#!/bin/bash
# Create macOS .app bundle from built binary
# Usage: ./create-app.sh <version> <binary-path> [icon-path]
set -euo pipefail

VERSION="${1:-0.1.0}"
BINARY="${2:-../../shuttle-gui}"
ICON="${3:-}"
APP_NAME="Shuttle.app"
BUNDLE_DIR="dist/${APP_NAME}"

echo "Creating ${APP_NAME} v${VERSION}..."

# Create bundle structure
mkdir -p "${BUNDLE_DIR}/Contents/MacOS"
mkdir -p "${BUNDLE_DIR}/Contents/Resources"

# Copy binary
cp "${BINARY}" "${BUNDLE_DIR}/Contents/MacOS/shuttle-gui"
chmod +x "${BUNDLE_DIR}/Contents/MacOS/shuttle-gui"

# Generate Info.plist with version
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
sed "s/\${VERSION}/${VERSION}/g" "${SCRIPT_DIR}/Info.plist" > "${BUNDLE_DIR}/Contents/Info.plist"

# Copy icon if provided
if [ -n "${ICON}" ] && [ -f "${ICON}" ]; then
    cp "${ICON}" "${BUNDLE_DIR}/Contents/Resources/AppIcon.icns"
fi

# Create PkgInfo
echo -n "APPL????" > "${BUNDLE_DIR}/Contents/PkgInfo"

echo "Created ${BUNDLE_DIR}"
echo ""
echo "To code sign:  codesign --deep --force --sign 'Developer ID Application: YOUR_NAME' ${BUNDLE_DIR}"
echo "To notarize:   xcrun notarytool submit ${BUNDLE_DIR} --apple-id YOUR_ID --team-id YOUR_TEAM"
echo "To create DMG: hdiutil create -volname Shuttle -srcfolder dist -ov -format UDZO dist/Shuttle-${VERSION}.dmg"
