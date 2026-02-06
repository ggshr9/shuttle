#!/bin/bash
#
# Common test utilities
#

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

# Wait for service to be ready
wait_for_service() {
    local host=$1
    local port=$2
    local timeout=${3:-30}
    local count=0

    while ! nc -z "$host" "$port" 2>/dev/null; do
        count=$((count + 1))
        if [ $count -ge $timeout ]; then
            error "Timeout waiting for $host:$port"
            return 1
        fi
        sleep 1
    done
    return 0
}

# Check HTTP endpoint
check_http() {
    local url=$1
    local timeout=${2:-5}

    curl -sf --max-time "$timeout" "$url" > /dev/null 2>&1
}

# Check API status
check_api_status() {
    local host=$1
    local port=${2:-9090}

    check_http "http://$host:$port/api/status"
}
