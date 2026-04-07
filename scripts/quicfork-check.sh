#!/bin/bash
# scripts/quicfork-check.sh
# Reports divergence between quicfork/ and upstream quic-go

set -e

UPSTREAM_VERSION="v0.59.0"

# Check go.mod for the replace directive to get the version
if grep -q "quic-go/quic-go" go.mod; then
    UPSTREAM_VERSION=$(grep "quic-go/quic-go" go.mod | grep -v replace | awk '{print $2}')
    echo "Upstream version from go.mod: $UPSTREAM_VERSION"
fi

TEMP_DIR=$(mktemp -d)
trap "rm -rf $TEMP_DIR" EXIT

echo "Fetching upstream quic-go $UPSTREAM_VERSION..."
git clone --depth=1 --branch="$UPSTREAM_VERSION" \
  https://github.com/quic-go/quic-go.git "$TEMP_DIR/upstream" 2>/dev/null

echo ""
echo "=== Shuttle-specific additions ==="
for f in quicfork/congestion_hook.go; do
    if [ -f "$f" ]; then
        echo "  $f ($(wc -l < "$f" | tr -d ' ') lines)"
    fi
done

echo ""
echo "=== Modified upstream files ==="
diff_count=0
while IFS= read -r file; do
    rel="${file#quicfork/}"
    upstream="$TEMP_DIR/upstream/$rel"
    if [ -f "$upstream" ]; then
        if ! diff -q "$file" "$upstream" >/dev/null 2>&1; then
            lines=$(diff "$file" "$upstream" 2>/dev/null | grep -c '^[<>]' || true)
            echo "  $rel ($lines lines changed)"
            diff_count=$((diff_count + 1))
        fi
    fi
done < <(find quicfork -name '*.go' -not -name '*_test.go' | sort)

echo ""
echo "=== Summary ==="
echo "Upstream: quic-go $UPSTREAM_VERSION"
echo "Modified files: $diff_count"
echo ""
echo "To check for CVEs: go vuln check github.com/quic-go/quic-go@$UPSTREAM_VERSION"
