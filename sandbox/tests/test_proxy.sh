#!/bin/bash
#
# Proxy functionality tests
#

set -e

source "$(dirname "$0")/common.sh"

test_socks5_basic() {
    log "Testing SOCKS5 basic connectivity..."

    # Test through client-a
    if curl -sf --socks5 127.0.0.1:1080 --max-time 10 http://httpbin.org/ip; then
        success "SOCKS5 basic test passed"
        return 0
    else
        error "SOCKS5 basic test failed"
        return 1
    fi
}

test_http_basic() {
    log "Testing HTTP proxy basic connectivity..."

    if curl -sf --proxy http://127.0.0.1:8080 --max-time 10 http://httpbin.org/ip; then
        success "HTTP proxy basic test passed"
        return 0
    else
        error "HTTP proxy basic test failed"
        return 1
    fi
}

test_https_through_proxy() {
    log "Testing HTTPS through proxy..."

    if curl -sf --socks5 127.0.0.1:1080 --max-time 10 https://httpbin.org/ip; then
        success "HTTPS through proxy test passed"
        return 0
    else
        error "HTTPS through proxy test failed"
        return 1
    fi
}

# Run all tests
main() {
    test_socks5_basic
    test_http_basic
    test_https_through_proxy
}

main "$@"
