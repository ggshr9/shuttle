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
#   ./scripts/test.sh --race       # Host tests with race detector
#   ./scripts/test.sh --cover      # Host tests with coverage report
#   ./scripts/test.sh --bench      # Run benchmarks
#   ./scripts/test.sh --fuzz SECS  # Run fuzz tests for N seconds (default 30)
#   ./scripts/test.sh --bg         # Run in background, log to file
#   ./scripts/test.sh --watch      # Watch mode: re-run on file changes

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

# Prevent toolchain download in offline environments
export GOTOOLCHAIN=local

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

log() { echo -e "${BLUE}[test]${NC} $1"; }
success() { echo -e "${GREEN}[✓]${NC} $1"; }
error() { echo -e "${RED}[✗]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
info() { echo -e "${CYAN}[i]${NC} $1"; }

MODE="host"
PKG="./..."
RUN_FILTER=""
RACE_FLAG=""
COVER_FLAG=""
BENCH_MODE=false
FUZZ_MODE=false
FUZZ_SECS=30
BG_MODE=false
WATCH_MODE=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --all)      MODE="all"; shift ;;
        --sandbox)  MODE="sandbox"; shift ;;
        --pkg)      PKG="$2"; shift 2 ;;
        --run)      RUN_FILTER="-run $2"; shift 2 ;;
        --race)     RACE_FLAG="-race"; shift ;;
        --cover)    COVER_FLAG="-coverprofile=coverage.out -covermode=atomic"; shift ;;
        --bench)    BENCH_MODE=true; shift ;;
        --fuzz)     FUZZ_MODE=true; FUZZ_SECS="${2:-30}"; shift; [[ "$1" =~ ^[0-9]+$ ]] && shift ;;
        --bg)       BG_MODE=true; shift ;;
        --watch)    WATCH_MODE=true; shift ;;
        -h|--help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Modes:"
            echo "  (default)     Host-safe unit tests only"
            echo "  --all         Host tests + Docker sandbox integration tests"
            echo "  --sandbox     Docker sandbox tests only"
            echo ""
            echo "Options:"
            echo "  --pkg PKG     Test specific Go package (default: ./...)"
            echo "  --run REGEX   Run tests matching regex"
            echo "  --race        Enable Go race detector"
            echo "  --cover       Generate coverage report (coverage.out + coverage.html)"
            echo "  --bench       Run benchmark tests"
            echo "  --fuzz SECS   Run fuzz tests for N seconds (default: 30)"
            echo "  --bg          Run in background, output to test.log"
            echo "  --watch       Watch mode: re-run on .go file changes"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

# Background mode: re-exec with output redirected
if $BG_MODE; then
    LOG_FILE="$PROJECT_DIR/test.log"
    info "Running tests in background. Output: $LOG_FILE"
    info "Tail with: tail -f $LOG_FILE"
    # Strip --bg from args and re-exec
    ARGS=()
    for arg in "$@"; do
        [[ "$arg" != "--bg" ]] && ARGS+=("$arg")
    done
    nohup "$0" "${ARGS[@]}" > "$LOG_FILE" 2>&1 &
    BG_PID=$!
    echo "$BG_PID" > "$PROJECT_DIR/.test.pid"
    info "PID: $BG_PID (saved to .test.pid)"
    exit 0
fi

HOST_RESULT=0
SANDBOX_RESULT=0

# ============================================
# Host-safe unit tests
# ============================================
run_host_tests() {
    log "Running host-safe unit tests..."
    [[ -n "$RACE_FLAG" ]] && info "Race detector: ON"
    [[ -n "$COVER_FLAG" ]] && info "Coverage: ON"
    echo ""

    START_TIME=$(date +%s)

    if go test -count=1 -v -timeout 120s $RACE_FLAG $COVER_FLAG $RUN_FILTER "$PKG" 2>&1; then
        END_TIME=$(date +%s)
        ELAPSED=$((END_TIME - START_TIME))
        success "Host tests passed (${ELAPSED}s)"
    else
        HOST_RESULT=1
        END_TIME=$(date +%s)
        ELAPSED=$((END_TIME - START_TIME))
        error "Host tests failed (${ELAPSED}s)"
    fi

    # Generate HTML coverage report if coverage was enabled
    if [[ -n "$COVER_FLAG" ]] && [[ -f coverage.out ]]; then
        go tool cover -func=coverage.out | tail -1
        go tool cover -html=coverage.out -o coverage.html 2>/dev/null && \
            info "Coverage report: coverage.html"
    fi

    echo ""
}

# ============================================
# Benchmark tests
# ============================================
run_benchmarks() {
    log "Running benchmark tests..."
    echo ""

    if go test -bench=. -benchmem -timeout 120s -run='^$' "$PKG" 2>&1; then
        success "Benchmarks completed"
    else
        HOST_RESULT=1
        error "Benchmarks failed"
    fi
    echo ""
}

# ============================================
# Fuzz tests
# ============================================
run_fuzz_tests() {
    log "Running fuzz tests (${FUZZ_SECS}s each)..."
    echo ""

    FUZZ_RESULT=0

    # Find all fuzz functions
    FUZZ_FUNCS=$(grep -r 'func Fuzz' --include='*_test.go' -l "$PROJECT_DIR" 2>/dev/null || true)

    if [[ -z "$FUZZ_FUNCS" ]]; then
        warn "No fuzz tests found"
        return
    fi

    for fuzz_file in $FUZZ_FUNCS; do
        fuzz_pkg=$(dirname "$fuzz_file" | sed "s|$PROJECT_DIR|.|")
        fuzz_names=$(grep -oP 'func (Fuzz\w+)' "$fuzz_file" | sed 's/func //')

        for fuzz_name in $fuzz_names; do
            info "Fuzzing $fuzz_pkg/$fuzz_name..."
            if go test -fuzz="^${fuzz_name}$" -fuzztime="${FUZZ_SECS}s" -timeout $((FUZZ_SECS + 30))s "$fuzz_pkg" 2>&1; then
                success "$fuzz_name passed"
            else
                FUZZ_RESULT=1
                error "$fuzz_name found issues"
            fi
        done
    done

    if [[ $FUZZ_RESULT -ne 0 ]]; then
        HOST_RESULT=1
        error "Some fuzz tests found issues"
    else
        success "All fuzz tests passed"
    fi
    echo ""
}

# ============================================
# Watch mode
# ============================================
run_watch() {
    log "Watch mode — re-running tests on .go file changes"
    info "Press Ctrl+C to stop"
    echo ""

    LAST_HASH=""
    while true; do
        # Compute hash of all .go files
        CUR_HASH=$(find "$PROJECT_DIR" -name '*.go' -newer "$PROJECT_DIR/.test.stamp" 2>/dev/null | head -1)

        if [[ -n "$CUR_HASH" ]] || [[ ! -f "$PROJECT_DIR/.test.stamp" ]]; then
            touch "$PROJECT_DIR/.test.stamp"
            echo ""
            log "Change detected, running tests..."
            run_host_tests
        fi
        sleep 2
    done
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
    docker compose -f sandbox/docker-compose.yml build 2>&1 | tail -5
    docker compose -f sandbox/docker-compose.yml --profile gotest build gotest 2>&1 | tail -3

    # Start full infrastructure (e2e tests need clients + httpbin)
    docker compose -f sandbox/docker-compose.yml up -d server router httpbin client-a client-b stun 2>&1

    # Wait for services
    sleep 5

    # Run tests
    if docker compose -f sandbox/docker-compose.yml --profile gotest run --rm gotest 2>&1; then
        success "Sandbox tests passed"
    else
        SANDBOX_RESULT=1
        error "Sandbox tests failed"
    fi

    # Cleanup
    docker compose -f sandbox/docker-compose.yml --profile gotest --profile test down -v 2>&1 | tail -1

    echo ""
}

# ============================================
# Main
# ============================================

# Watch mode overrides everything
if $WATCH_MODE; then
    run_watch
    exit 0
fi

# Fuzz mode
if $FUZZ_MODE; then
    run_fuzz_tests
    exit $HOST_RESULT
fi

# Bench mode
if $BENCH_MODE; then
    run_benchmarks
    exit $HOST_RESULT
fi

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
