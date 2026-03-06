#!/bin/bash
#
# Unified test runner for Shuttle.
#
# Runs host-safe unit tests, then optionally runs Docker sandbox
# integration tests (STUN, NAT, mDNS, hole punching, etc.)
#
# Usage:
#   ./scripts/test.sh              # Host tests only (fast, safe)
#   ./scripts/test.sh --all        # Host tests + Docker sandbox tests
#   ./scripts/test.sh --sandbox    # Docker sandbox tests only
#   ./scripts/test.sh --pkg PKG    # Test a specific package on host
#   ./scripts/test.sh --run REGEX  # Run matching tests on host

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log() { echo -e "${BLUE}[test]${NC} $1"; }
success() { echo -e "${GREEN}[✓]${NC} $1"; }
error() { echo -e "${RED}[✗]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }

MODE="host"
PKG="./..."
RUN_FILTER=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --all)      MODE="all"; shift ;;
        --sandbox)  MODE="sandbox"; shift ;;
        --pkg)      PKG="$2"; shift 2 ;;
        --run)      RUN_FILTER="-run $2"; shift 2 ;;
        -h|--help)
            echo "Usage: $0 [--all|--sandbox] [--pkg PKG] [--run REGEX]"
            echo ""
            echo "  (default)   Host-safe unit tests only"
            echo "  --all       Host tests + Docker sandbox integration tests"
            echo "  --sandbox   Docker sandbox tests only"
            echo "  --pkg PKG   Test specific Go package (default: ./...)"
            echo "  --run REGEX Run tests matching regex"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

HOST_RESULT=0
SANDBOX_RESULT=0

# ============================================
# Host-safe unit tests
# ============================================
run_host_tests() {
    log "Running host-safe unit tests..."
    echo ""

    if go test -count=1 -v -timeout 120s $RUN_FILTER "$PKG" 2>&1; then
        success "Host tests passed"
    else
        HOST_RESULT=1
        error "Host tests failed"
    fi
    echo ""
}

# ============================================
# Docker sandbox integration tests
# ============================================
run_sandbox_tests() {
    # Check Docker
    if ! docker info >/dev/null 2>&1; then
        warn "Docker is not running. Starting Docker Desktop..."
        open -a Docker 2>/dev/null || true

        local tries=0
        while ! docker info >/dev/null 2>&1; do
            sleep 2
            ((tries++))
            if [ $tries -ge 30 ]; then
                error "Docker failed to start after 60s. Skipping sandbox tests."
                SANDBOX_RESULT=1
                return
            fi
        done
        success "Docker started"
    fi

    log "Running Docker sandbox integration tests..."
    echo ""

    # Build images if needed
    docker compose -f sandbox/docker-compose.yml build stun 2>&1 | tail -3
    docker compose -f sandbox/docker-compose.yml --profile gotest build gotest 2>&1 | tail -3

    # Start infrastructure
    docker compose -f sandbox/docker-compose.yml up -d stun router 2>&1

    # Wait for services
    sleep 2

    # Run tests
    if docker compose -f sandbox/docker-compose.yml --profile gotest run --rm gotest 2>&1; then
        success "Sandbox tests passed"
    else
        SANDBOX_RESULT=1
        error "Sandbox tests failed"
    fi

    # Cleanup
    docker compose -f sandbox/docker-compose.yml --profile gotest down -v 2>&1 | tail -1

    echo ""
}

# ============================================
# Main
# ============================================
case "$MODE" in
    host)
        run_host_tests
        exit $HOST_RESULT
        ;;
    sandbox)
        run_sandbox_tests
        exit $SANDBOX_RESULT
        ;;
    all)
        run_host_tests
        run_sandbox_tests
        if [ $HOST_RESULT -ne 0 ] || [ $SANDBOX_RESULT -ne 0 ]; then
            error "Some tests failed (host=$HOST_RESULT, sandbox=$SANDBOX_RESULT)"
            exit 1
        fi
        success "All tests passed"
        exit 0
        ;;
esac
