#!/usr/bin/env bash
#
# WebRTC DataChannel Transport — Test Runner
# ============================================
#
# Usage:
#   ./test/run_webrtc_tests.sh          # run all WebRTC tests
#   ./test/run_webrtc_tests.sh quick    # only unit tests (no network)
#   ./test/run_webrtc_tests.sh e2e      # only e2e tests (loopback network)
#   ./test/run_webrtc_tests.sh <name>   # run specific test by name substring
#
# All tests are self-contained:
#   - 127.0.0.1 + random ports only
#   - No external STUN/TURN
#   - mDNS disabled, loopback-only ICE
#   - No system proxy/routing/DNS changes
#
# Last updated: 2026-02-05
#   - Phase 1-3: ICEPolicy, stats, WS signaling, Trickle ICE, reconnect
#   - LoopbackOnly isolation (no mDNS, no external network)
#

set -euo pipefail
cd "$(dirname "$0")/.."

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

MODE="${1:-all}"

# ── Unit tests (no network at all) ──────────────────────────────────────────
UNIT_TESTS=(
    TestWebRTCClientCreate
    TestWebRTCServerCreate
    TestWebRTCClientDialClosed
    TestWebRTCSignalAuth
    TestWebRTCReplayRejection
)

# ── E2E tests (loopback network only) ──────────────────────────────────────
E2E_TESTS=(
    TestWebRTCEndToEnd
    TestWebRTCWSSignaling
    TestWebRTCTrickleICE
    TestWebRTCMultiStream
    TestWebRTCLargeTransfer
    TestWebRTCAuthFailureWS
    TestWebRTCICEPolicyRelay
    TestWebRTCConnectionStats
    TestWebRTCServerClose
)

join_pipe() {
    local IFS='|'
    echo "$*"
}

run_tests() {
    local label="$1"
    shift
    local pattern
    pattern=$(join_pipe "$@")

    echo -e "${YELLOW}▶ ${label}${NC}"
    echo "  Pattern: ${pattern}"
    echo ""

    if CGO_ENABLED=0 go test -count=1 -v ./test/ -run "^(${pattern})$" -timeout 120s; then
        echo -e "${GREEN}✓ ${label} — PASSED${NC}"
        echo ""
        return 0
    else
        echo -e "${RED}✗ ${label} — FAILED${NC}"
        echo ""
        return 1
    fi
}

echo "============================================"
echo " WebRTC Transport Tests"
echo "============================================"
echo ""

# ── Step 0: Compile check ──────────────────────────────────────────────────
echo -e "${YELLOW}▶ Compile check${NC}"
if CGO_ENABLED=0 go build ./transport/webrtc/ ./config/ ./engine/ ./cmd/shuttled/ ./cmd/shuttle/ 2>&1; then
    echo -e "${GREEN}✓ Compile check — PASSED${NC}"
    echo ""
else
    echo -e "${RED}✗ Compile check — FAILED${NC}"
    exit 1
fi

FAILED=0

case "$MODE" in
    quick)
        run_tests "Unit Tests" "${UNIT_TESTS[@]}" || FAILED=1
        ;;
    e2e)
        run_tests "E2E Tests (loopback)" "${E2E_TESTS[@]}" || FAILED=1
        ;;
    all)
        run_tests "Unit Tests" "${UNIT_TESTS[@]}" || FAILED=1
        run_tests "E2E Tests (loopback)" "${E2E_TESTS[@]}" || FAILED=1
        ;;
    *)
        # Treat as test name filter
        echo -e "${YELLOW}▶ Running tests matching: ${MODE}${NC}"
        CGO_ENABLED=0 go test -count=1 -v ./test/ -run "${MODE}" -timeout 120s || FAILED=1
        ;;
esac

echo "============================================"
if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN} ALL PASSED${NC}"
else
    echo -e "${RED} SOME TESTS FAILED${NC}"
fi
echo "============================================"

exit $FAILED
