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

# Flag any `>Title Case English Words<` or `placeholder="English words"` etc.
# Allow-listed: test/spec files; rely on review for edge cases.
if grep -rEn \
     --include='*.svelte' \
     '(>[A-Z][a-z]+ [A-Z][a-z][^<{]+<|placeholder="[A-Z][a-z][^"]+"|aria-label="[A-Z][a-z][^"]+")' \
     "${targets[@]}" 2>/dev/null | grep -vE '(\.test\.|\.spec\.)' | head -5; then
  echo ""
  echo "❌ Found apparent hardcoded English in ui/ or features/. Use t(key) via @/lib/i18n."
  exit 1
fi

echo "✓ No hardcoded English found in ui/ or features/"
