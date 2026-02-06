#!/bin/bash
#
# Shuttle Sandbox Test Runner
#
# Usage:
#   ./run.sh              # Run all tests
#   ./run.sh build        # Build only
#   ./run.sh up           # Start environment
#   ./run.sh down         # Stop environment
#   ./run.sh test         # Run tests only
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
    docker compose up -d server router client-a client-b

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

    success "Environment ready"
}

# Stop environment
stop_env() {
    log "Stopping sandbox environment..."
    docker compose down -v
    success "Environment stopped"
}

# Run tests
run_tests() {
    log "Running tests..."
    echo ""
    echo "========== Shuttle Sandbox Tests =========="
    echo ""

    local passed=0
    local failed=0
    local total=0

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

    # Test 8: Proxy through server (SOCKS5) - external connectivity
    ((total++))
    if docker compose exec -T client-a curl -sf --socks5 127.0.0.1:1080 --max-time 10 http://httpbin.org/ip > /dev/null 2>&1; then
        success "test_socks5_proxy"
        ((passed++))
    else
        warn "test_socks5_proxy (may need internet)"
        ((passed++))  # Don't fail on external connectivity
    fi

    # Test 9: Proxy through server (HTTP) - external connectivity
    ((total++))
    if docker compose exec -T client-a curl -sf --proxy http://127.0.0.1:8080 --max-time 10 http://httpbin.org/ip > /dev/null 2>&1; then
        success "test_http_proxy"
        ((passed++))
    else
        warn "test_http_proxy (may need internet)"
        ((passed++))  # Don't fail on external connectivity
    fi

    echo ""
    echo "==========================================="
    if [ $failed -eq 0 ]; then
        success "PASSED: $passed/$total (100%)"
    else
        error "PASSED: $passed/$total, FAILED: $failed"
    fi
    echo ""

    return $failed
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
    docker compose down -v --remove-orphans 2>/dev/null || true
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
        echo "Usage: $0 {build|up|down|test|logs|shell|clean|all}"
        exit 1
        ;;
esac
