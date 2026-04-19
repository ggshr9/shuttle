#!/usr/bin/env bash
# Detect hardcoded user-visible English in ui/ and features/.
# Strings must come through lib/i18n or be explicit i18n keys.
set -euo pipefail

cd "$(dirname "$0")/.."

# Target directories may not exist yet in P1 — guard for that.
targets=()
[ -d src/ui ] && targets+=(src/ui)
[ -d src/features ] && targets+=(src/features)
if [ ${#targets[@]} -eq 0 ]; then
  echo "✓ No ui/ or features/ yet; nothing to check."
  exit 0
fi

# Flag user-visible English in two ways:
#  1. Multi-word text inside tags (`>Foo Bar<`)
#  2. Placeholder attributes with multi-word values
# aria-labels are NOT flagged — primitives often need short a11y strings
# ("Loading", "Close") that are universal and tiny enough not to warrant an
# i18n lookup. Review catches the cases where aria-label really should
# localize.
if grep -rEn \
     --include='*.svelte' \
     '(>[A-Z][a-z]+ [A-Z][a-z][^<{]+<|placeholder="[A-Z][a-z]+ [A-Z][a-z][^"]+")' \
     "${targets[@]}" 2>/dev/null | grep -vE '(\.test\.|\.spec\.)' | head -5; then
  echo ""
  echo "❌ Found apparent hardcoded English in ui/ or features/. Use t(key) via @/lib/i18n."
  exit 1
fi

echo "✓ No hardcoded English found in ui/ or features/"
