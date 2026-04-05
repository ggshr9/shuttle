#!/usr/bin/env bash
# scripts/quicfork-diff.sh — Show divergence between quicfork/ and upstream quic-go.
#
# Usage: ./scripts/quicfork-diff.sh [upstream-tag]
# Default upstream tag is read from go.mod's replace directive.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Extract the upstream version from go.mod replace directive.
# The line looks like: github.com/quic-go/quic-go v0.59.0 => ./quicfork
UPSTREAM_VERSION="${1:-$(grep 'quic-go/quic-go' "$ROOT/go.mod" | grep -v '=>' | awk '{print $2}')}"
if [[ -z "$UPSTREAM_VERSION" ]]; then
    echo "ERROR: Could not determine upstream quic-go version from go.mod"
    exit 1
fi

TAG="${UPSTREAM_VERSION}"
echo "=== Comparing quicfork/ against upstream quic-go $TAG ==="

TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

# Download upstream source.
echo "Fetching upstream quic-go@$TAG..."
GOPATH="$TMPDIR/gopath" go mod download "github.com/quic-go/quic-go@$TAG" 2>/dev/null || {
    echo "ERROR: Failed to download upstream quic-go@$TAG"
    exit 1
}

UPSTREAM_DIR=$(GOPATH="$TMPDIR/gopath" go env GOMODCACHE)/github.com/quic-go/quic-go@"$TAG"

if [[ ! -d "$UPSTREAM_DIR" ]]; then
    echo "ERROR: Upstream directory not found at $UPSTREAM_DIR"
    exit 1
fi

echo ""
echo "=== Files only in quicfork/ (custom additions) ==="
diff -rq "$UPSTREAM_DIR" "$ROOT/quicfork/" 2>/dev/null | grep "Only in $ROOT" | head -20 || echo "(none)"

echo ""
echo "=== Files only in upstream (removed from fork) ==="
diff -rq "$UPSTREAM_DIR" "$ROOT/quicfork/" 2>/dev/null | grep "Only in $UPSTREAM_DIR" | head -20 || echo "(none)"

echo ""
echo "=== Files modified from upstream ==="
diff -rq "$UPSTREAM_DIR" "$ROOT/quicfork/" 2>/dev/null | grep "^Files" | head -30 || echo "(none)"

echo ""
echo "=== Summary ==="
TOTAL_DIFF=$(diff -r "$UPSTREAM_DIR" "$ROOT/quicfork/" 2>/dev/null | wc -l | tr -d ' ')
echo "Total diff lines: $TOTAL_DIFF"
echo ""
echo "For full diff: diff -r \"$UPSTREAM_DIR\" \"$ROOT/quicfork/\""
