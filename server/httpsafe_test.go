package server

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestSafeDialContext_BlocksLiteralLoopback(t *testing.T) {
	dial := SafeDialContext(false)
	_, err := dial(context.Background(), "tcp", "127.0.0.1:80")
	if !errors.Is(err, ErrBlockedTarget) {
		t.Fatalf("expected ErrBlockedTarget, got %v", err)
	}
}

func TestSafeDialContext_BlocksLiteralPrivate(t *testing.T) {
	dial := SafeDialContext(false)
	for _, addr := range []string{"10.0.0.1:80", "192.168.1.1:443", "169.254.169.254:80", "[::1]:80"} {
		_, err := dial(context.Background(), "tcp", addr)
		if !errors.Is(err, ErrBlockedTarget) {
			t.Errorf("addr %q: expected ErrBlockedTarget, got %v", addr, err)
		}
	}
}

func TestSafeDialContext_AllowPrivateBypass(t *testing.T) {
	// With AllowPrivate=true, loopback should be attempted. We don't bind a
	// listener, so we expect a connection-refused error, NOT ErrBlockedTarget.
	dial := SafeDialContext(true)
	_, err := dial(context.Background(), "tcp", "127.0.0.1:1") // port 1 unlikely to be bound
	if errors.Is(err, ErrBlockedTarget) {
		t.Errorf("AllowPrivate should bypass SSRF check, got ErrBlockedTarget")
	}
	// err may be non-nil (connection refused) — that's fine.
	_ = err
}

var _ = net.IPv4 // keep import
