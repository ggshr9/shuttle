#!/usr/bin/env bash
# Cloud E2E Test Runner
#
# Runs the full e2e test suite on localhost without Docker.
# Uses loopback addresses (127.0.0.2, 127.0.0.3) to simulate separate hosts.
#
# Usage:
#   ./test/cloud/run.sh              # Build, start services, run tests, cleanup
#   ./test/cloud/run.sh build        # Build binaries only
#   ./test/cloud/run.sh start        # Start services (background)
#   ./test/cloud/run.sh test         # Run e2e tests (services must be running)
#   ./test/cloud/run.sh stop         # Stop services
#   ./test/cloud/run.sh logs         # Show service logs

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
BIN_DIR="$SCRIPT_DIR/bin"
LOG_DIR="$SCRIPT_DIR/logs"
PID_DIR="$SCRIPT_DIR/pids"

# Service addresses
# Bind httpbin to the host's routable IP so the server's SSRF protection
# doesn't block it (127.0.0.0/8 and 10.0.0.0/8 are blocked).
CLOUD_IP=$(hostname -I | awk '{print $1}')
export HTTPBIN_ADDR="${CLOUD_IP}:18080"
export SANDBOX_SERVER_ADDR="127.0.0.1:10443"
export SANDBOX_HTTPBIN_ADDR="${CLOUD_IP}:18080"
export SANDBOX_CLIENT_A_ADDR="127.0.0.2"
export SANDBOX_CLIENT_B_ADDR="127.0.0.3"
export SANDBOX_CLIENT_A_API="127.0.0.2:9090"
export SANDBOX_CLIENT_B_API="127.0.0.3:9090"

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { echo -e "${BLUE}[cloud]${NC} $*"; }
pass()  { echo -e "${GREEN}[✓]${NC} $*"; }
fail()  { echo -e "${RED}[✗]${NC} $*"; }

cleanup() {
    info "Cleaning up..."
    for pidfile in "$PID_DIR"/*.pid; do
        [ -f "$pidfile" ] || continue
        pid=$(cat "$pidfile")
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null || true
            wait "$pid" 2>/dev/null || true
        fi
        rm -f "$pidfile"
    done
    info "Done."
}

do_build() {
    info "Building binaries..."
    mkdir -p "$BIN_DIR"
    (cd "$PROJECT_ROOT" && GOTOOLCHAIN=local CGO_ENABLED=0 go build -o "$BIN_DIR/shuttled" ./cmd/shuttled)
    (cd "$PROJECT_ROOT" && GOTOOLCHAIN=local CGO_ENABLED=0 go build -o "$BIN_DIR/shuttle"  ./cmd/shuttle)
    (cd "$PROJECT_ROOT" && GOTOOLCHAIN=local CGO_ENABLED=0 go build -o "$BIN_DIR/httpbin"  ./test/cloud/httpbin)
    pass "Binaries built"
}

wait_for_port() {
    local addr=$1 timeout=${2:-15}
    local deadline=$((SECONDS + timeout))
    while [ $SECONDS -lt $deadline ]; do
        if bash -c "echo >/dev/tcp/${addr%:*}/${addr#*:}" 2>/dev/null; then
            return 0
        fi
        sleep 0.5
    done
    return 1
}

do_start() {
    mkdir -p "$LOG_DIR" "$PID_DIR"

    # 1. httpbin
    info "Starting httpbin on $HTTPBIN_ADDR..."
    "$BIN_DIR/httpbin" > "$LOG_DIR/httpbin.log" 2>&1 &
    echo $! > "$PID_DIR/httpbin.pid"
    wait_for_port "$HTTPBIN_ADDR" 5 || { fail "httpbin failed to start"; cat "$LOG_DIR/httpbin.log"; return 1; }
    pass "httpbin ready"

    # 2. shuttled (server)
    info "Starting shuttled on $SANDBOX_SERVER_ADDR..."
    "$BIN_DIR/shuttled" run -c "$SCRIPT_DIR/configs/server.yaml" > "$LOG_DIR/server.log" 2>&1 &
    echo $! > "$PID_DIR/server.pid"
    sleep 2
    if ! kill -0 "$(cat "$PID_DIR/server.pid")" 2>/dev/null; then
        fail "shuttled failed to start"
        cat "$LOG_DIR/server.log"
        return 1
    fi
    pass "shuttled ready"

    # 3. shuttle client-a (API mode)
    info "Starting client-a on $SANDBOX_CLIENT_A_ADDR..."
    "$BIN_DIR/shuttle" api \
        -c "$SCRIPT_DIR/configs/client-a.yaml" \
        --listen "$SANDBOX_CLIENT_A_API" \
        --auto-connect \
        > "$LOG_DIR/client-a.log" 2>&1 &
    echo $! > "$PID_DIR/client-a.pid"
    wait_for_port "$SANDBOX_CLIENT_A_API" 15 || { fail "client-a API failed to start"; cat "$LOG_DIR/client-a.log"; return 1; }
    pass "client-a ready (API=$SANDBOX_CLIENT_A_API, SOCKS5=$SANDBOX_CLIENT_A_ADDR:1080)"

    # 4. shuttle client-b (API mode)
    info "Starting client-b on $SANDBOX_CLIENT_B_ADDR..."
    "$BIN_DIR/shuttle" api \
        -c "$SCRIPT_DIR/configs/client-b.yaml" \
        --listen "$SANDBOX_CLIENT_B_API" \
        --auto-connect \
        > "$LOG_DIR/client-b.log" 2>&1 &
    echo $! > "$PID_DIR/client-b.pid"
    wait_for_port "$SANDBOX_CLIENT_B_API" 15 || { fail "client-b API failed to start"; cat "$LOG_DIR/client-b.log"; return 1; }
    pass "client-b ready (API=$SANDBOX_CLIENT_B_API, SOCKS5=$SANDBOX_CLIENT_B_ADDR:1080)"

    # 5. Wait for proxy ports
    info "Waiting for proxy ports..."
    sleep 2
    local ok=true
    for addr in "$SANDBOX_CLIENT_A_ADDR:1080" "$SANDBOX_CLIENT_A_ADDR:8080" \
                "$SANDBOX_CLIENT_B_ADDR:1080" "$SANDBOX_CLIENT_B_ADDR:8080"; do
        if wait_for_port "$addr" 10; then
            pass "  $addr"
        else
            fail "  $addr not ready"
            ok=false
        fi
    done

    if $ok; then
        pass "All services ready"
    else
        fail "Some services failed to start — check logs in $LOG_DIR/"
        return 1
    fi
}

do_test() {
    info "Running e2e tests..."
    (cd "$PROJECT_ROOT" && GOTOOLCHAIN=local go test \
        -tags sandbox \
        -v \
        -count=1 \
        -timeout 120s \
        ./test/e2e/ \
    ) 2>&1 | tee "$LOG_DIR/test-output.log"

    local exit_code=${PIPESTATUS[0]}
    echo ""
    if [ $exit_code -eq 0 ]; then
        pass "E2E tests passed!"
    else
        fail "E2E tests failed (exit code $exit_code)"
        info "Service logs available in $LOG_DIR/"
    fi
    return $exit_code
}

do_stop() {
    cleanup
}

do_logs() {
    for log in "$LOG_DIR"/*.log; do
        [ -f "$log" ] || continue
        echo "=== $(basename "$log") ==="
        tail -20 "$log"
        echo ""
    done
}

# ---- Main ----
case "${1:-all}" in
    build)
        do_build
        ;;
    start|up)
        do_start
        ;;
    test)
        do_test
        ;;
    stop|down)
        do_stop
        ;;
    logs)
        do_logs
        ;;
    all)
        trap cleanup EXIT
        do_build
        do_start
        do_test
        ;;
    *)
        echo "Usage: $0 {all|build|start|test|stop|logs}"
        exit 1
        ;;
esac
