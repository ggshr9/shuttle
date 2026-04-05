#!/bin/bash
#
# Shuttle Sandbox Test Runner
#
# Usage:
#   ./run.sh              # Run all tests (shell)
#   ./run.sh build        # Build only
#   ./run.sh up           # Start environment
#   ./run.sh down         # Stop environment
#   ./run.sh test         # Run shell tests only
#   ./run.sh gotest       # Run Go integration tests in Docker
#   ./run.sh logs         # View logs
#   ./run.sh shell <name> # Shell into container

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$SCRIPT_DIR"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log() { echo -e "${BLUE}[sandbox]${NC} $1"; }
success() { echo -e "${GREEN}[✓]${NC} $1"; }
error() { echo -e "${RED}[✗]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }

# Build binaries
build_binaries() {
    log "Building shuttle binaries..."
    cd "$PROJECT_DIR"

    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o sandbox/shuttle ./cmd/shuttle
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o sandbox/shuttled ./cmd/shuttled

    success "Binaries built"
    cd "$SCRIPT_DIR"
}

# Build Docker images
build_images() {
    log "Building Docker images..."
    docker compose build
    success "Images built"
}

# Start environment
start_env() {
    log "Starting sandbox environment..."
    docker compose up -d server router httpbin client-a client-b stun

    log "Waiting for services to be ready..."
    sleep 5

    # Check health - verify shuttled process is running
    if docker compose exec -T server pgrep -x shuttled > /dev/null 2>&1; then
        success "Server is healthy"
    else
        error "Server health check failed"
        docker compose logs server
        exit 1
    fi

    # Check STUN server
    if docker compose exec -T stun pgrep -x stunserver > /dev/null 2>&1; then
        success "STUN server is healthy"
    else
        warn "STUN server not running (gotest tests may fail)"
    fi

    success "Environment ready"
}

# Stop environment
stop_env() {
    log "Stopping sandbox environment..."
    docker compose --profile gotest --profile test down -v
    success "Environment stopped"
}

# Run shell tests
run_tests() {
    log "Running tests..."
    echo ""
    echo "========== Shuttle Sandbox Tests =========="
    echo ""

    local passed=0
    local failed=0
    local total=0
    set +e  # arithmetic ((...)) returns 1 when result is 0; disable errexit for tests

    # Test 1: Server running
    ((total++))
    if docker compose exec -T server pgrep -x shuttled > /dev/null 2>&1; then
        success "test_server_running"
        ((passed++))
    else
        error "test_server_running"
        ((failed++))
    fi

    # Test 2: Client A running
    ((total++))
    if docker compose exec -T client-a pgrep -x shuttle > /dev/null 2>&1; then
        success "test_client_a_running"
        ((passed++))
    else
        error "test_client_a_running"
        ((failed++))
    fi

    # Test 3: Client B running
    ((total++))
    if docker compose exec -T client-b pgrep -x shuttle > /dev/null 2>&1; then
        success "test_client_b_running"
        ((passed++))
    else
        error "test_client_b_running"
        ((failed++))
    fi

    # Test 4: Client A -> Router connectivity (on net-a)
    ((total++))
    if docker compose exec -T client-a ping -c 1 -W 2 10.100.1.1 > /dev/null 2>&1; then
        success "test_client_a_to_router"
        ((passed++))
    else
        error "test_client_a_to_router"
        ((failed++))
    fi

    # Test 5: Client B -> Router connectivity (on net-b)
    ((total++))
    if docker compose exec -T client-b ping -c 1 -W 2 10.100.2.1 > /dev/null 2>&1; then
        success "test_client_b_to_router"
        ((passed++))
    else
        error "test_client_b_to_router"
        ((failed++))
    fi

    # Test 6: Client A -> Server connectivity (through router)
    ((total++))
    if docker compose exec -T client-a ping -c 1 -W 2 10.100.0.10 > /dev/null 2>&1; then
        success "test_client_a_to_server"
        ((passed++))
    else
        error "test_client_a_to_server"
        ((failed++))
    fi

    # Test 7: Proxy through server (SOCKS5) - via local httpbin
    ((total++))
    local socks5_result
    socks5_result=$(docker compose exec -T client-a curl -sf --socks5 127.0.0.1:1080 --max-time 10 http://10.100.0.20/ip 2>&1)
    if [ $? -eq 0 ] && echo "$socks5_result" | grep -q "origin"; then
        success "test_socks5_proxy (response: $(echo "$socks5_result" | tr -d '\n'))"
        ((passed++))
    else
        error "test_socks5_proxy"
        ((failed++))
    fi

    # Test 8: Proxy through server (HTTP) - via local httpbin
    ((total++))
    local http_result
    http_result=$(docker compose exec -T client-a curl -sf --proxy http://127.0.0.1:8080 --max-time 10 http://10.100.0.20/ip 2>&1)
    if [ $? -eq 0 ] && echo "$http_result" | grep -q "origin"; then
        success "test_http_proxy (response: $(echo "$http_result" | tr -d '\n'))"
        ((passed++))
    else
        error "test_http_proxy"
        ((failed++))
    fi

    # Test 9: Verify httpbin /get returns full request info
    ((total++))
    local get_result
    get_result=$(docker compose exec -T client-a curl -sf --socks5 127.0.0.1:1080 --max-time 10 http://10.100.0.20/get 2>&1)
    if [ $? -eq 0 ] && echo "$get_result" | grep -q "headers"; then
        success "test_socks5_get_endpoint"
        ((passed++))
    else
        error "test_socks5_get_endpoint"
        ((failed++))
    fi

    # Test 10: Client API status check
    ((total++))
    local api_status
    api_status=$(docker compose exec -T client-a curl -sf --max-time 5 http://127.0.0.1:9090/api/status 2>&1)
    if [ $? -eq 0 ] && echo "$api_status" | grep -q "state"; then
        success "test_client_a_api_status"
        ((passed++))
    else
        error "test_client_a_api_status"
        ((failed++))
    fi

    # Test 11: Client B SOCKS5 proxy (proxy_all mode)
    ((total++))
    if docker compose exec -T client-b curl -sf --socks5 127.0.0.1:1080 --max-time 10 http://10.100.0.20/ip > /dev/null 2>&1; then
        success "test_client_b_socks5"
        ((passed++))
    else
        error "test_client_b_socks5"
        ((failed++))
    fi

    echo ""
    echo "==========================================="
    if [ $failed -eq 0 ]; then
        success "PASSED: $passed/$total (100%)"
    else
        error "PASSED: $passed/$total, FAILED: $failed"
    fi
    echo ""

    set -e  # re-enable errexit
    return $failed
}

# Run Go integration tests inside Docker
run_gotest() {
    local test_pkg="${1:-}"
    local test_run="${2:-}"

    log "Running Go integration tests in Docker..."
    echo ""
    echo "========== Go Sandbox Tests =========="
    echo ""

    # Ensure full environment is up (e2e tests need clients + httpbin)
    docker compose up -d server router httpbin client-a client-b stun
    sleep 5

    # Wait for server health
    if docker compose exec -T server pgrep -x shuttled > /dev/null 2>&1; then
        success "Server is healthy"
    else
        error "Server health check failed"
        docker compose logs server
        return 1
    fi

    # Build and run gotest container
    local env_args=""
    if [ -n "$test_pkg" ]; then
        env_args="$env_args -e SANDBOX_TEST_PKG=$test_pkg"
    fi
    if [ -n "$test_run" ]; then
        env_args="$env_args -e SANDBOX_TEST_RUN=$test_run"
    fi

    docker compose --profile gotest build gotest
    docker compose --profile gotest run --rm $env_args gotest
    result=$?

    echo ""
    echo "======================================"
    if [ $result -eq 0 ]; then
        success "Go sandbox tests PASSED"
    else
        error "Go sandbox tests FAILED"
    fi
    echo ""

    return $result
}

# View logs
view_logs() {
    docker compose logs -f "$@"
}

# Shell into container
shell_into() {
    local name="${1:-server}"
    docker compose exec "$name" sh
}

# Cleanup
cleanup() {
    log "Cleaning up..."
    rm -f shuttle shuttled
    docker compose --profile gotest --profile test down -v --remove-orphans 2>/dev/null || true
    success "Cleaned up"
}

# Main
case "${1:-all}" in
    build)
        build_binaries
        build_images
        ;;
    up|start)
        build_binaries
        build_images
        start_env
        ;;
    down|stop)
        stop_env
        ;;
    test)
        run_tests
        ;;
    gotest)
        shift
        run_gotest "$@"
        ;;
    e2e)
        # Run only E2E tests
        run_gotest "./test/e2e/" "TestSandbox"
        ;;
    logs)
        shift
        view_logs "$@"
        ;;
    shell)
        shift
        shell_into "$@"
        ;;
    clean)
        cleanup
        ;;
    gui)
        # Start sandbox + open GUI for browser testing
        build_binaries
        build_images
        start_env
        echo ""
        success "Sandbox is running with GUI API exposed:"
        echo ""
        echo "  Client A API:  http://localhost:19091/api/status"
        echo "  Client B API:  http://localhost:19092/api/status"
        echo "  Server:        http://localhost:19080/api/health (admin)"
        echo ""
        echo "  To open GUI in browser:"
        echo "    cd gui/web && npm run dev:sandbox"
        echo "    Then open http://localhost:5174"
        echo ""
        echo "  Test full proxy chain:"
        echo '    curl -s localhost:19091/api/test/probe -d '"'"'{"url":"http://10.100.0.20/ip","via":"socks5"}'"'"' | jq .'
        echo ""
        echo "  Batch test:"
        echo '    curl -s localhost:19091/api/test/probe/batch -d '"'"'{"tests":[{"name":"socks5","url":"http://10.100.0.20/ip","via":"socks5"},{"name":"http","url":"http://10.100.0.20/ip","via":"http"},{"name":"direct","url":"http://10.100.0.20/ip","via":"direct"}]}'"'"' | jq .'
        echo ""
        log "Press Ctrl+C to stop, then run: $0 down"
        ;;
    dev)
        # Full dev environment: sandbox + mesh + GUI-ready
        build_binaries
        build_images
        start_env
        echo ""
        success "Dev environment ready (mesh enabled between clients):"
        echo ""
        echo "  Client A:  http://localhost:19091/api/status"
        echo "  Client B:  http://localhost:19092/api/status"
        echo "  Server:    http://localhost:19080/api/health"
        echo ""
        echo "  Mesh status:"
        echo "    curl -s localhost:19091/api/mesh/status | jq ."
        echo "    curl -s localhost:19092/api/mesh/status | jq ."
        echo ""
        echo "  Mesh peers:"
        echo "    curl -s localhost:19091/api/mesh/peers | jq ."
        echo ""
        echo "  Proxy test:"
        echo '    curl -s --socks5-hostname localhost:19091 http://10.100.0.20/ip'
        echo ""
        echo "  To start GUI dev server:"
        echo "    cd gui/web && npm run dev"
        echo "    Then open http://localhost:5173"
        echo ""
        log "Press Ctrl+C to stop, then run: $0 down"
        docker compose logs -f client-a client-b server 2>&1 | head -50
        ;;
    all|"")
        build_binaries
        build_images
        start_env
        run_tests
        result=$?
        stop_env
        exit $result
        ;;
    *)
        echo "Usage: $0 {build|up|down|test|gotest|e2e|gui|dev|logs|shell|clean|all}"
        exit 1
        ;;
esac
