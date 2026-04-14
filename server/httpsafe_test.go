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
	// listener, so we expect a network-level dial error, NOT ErrBlockedTarget.
	dial := SafeDialContext(true)
	_, err := dial(context.Background(), "tcp", "127.0.0.1:1") // port 1 unlikely to be bound
	if errors.Is(err, ErrBlockedTarget) {
		t.Fatalf("AllowPrivate should bypass SSRF check, got ErrBlockedTarget")
	}
	if err != nil {
		// Acceptable: a network-level error (connection refused, unreachable, etc.).
		// Unacceptable: any shape of error that doesn't originate from the dialer.
		var opErr *net.OpError
		if !errors.As(err, &opErr) {
			t.Fatalf("AllowPrivate non-nil error should be *net.OpError, got %T: %v", err, err)
		}
	}
}

func TestSafeDialContext_BlocksHostnameResolvingToPrivate(t *testing.T) {
	restore := setLookupIPAddr(func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	})
	defer restore()

	dial := SafeDialContext(false)
	_, err := dial(context.Background(), "tcp", "rebind.test:80")
	if !errors.Is(err, ErrBlockedTarget) {
		t.Fatalf("expected ErrBlockedTarget for hostname rebinding to loopback, got %v", err)
	}
}

func TestSafeDialContext_HostnameAllowsAfterFilteringBlocked(t *testing.T) {
	// Resolver returns [blocked, public]. Blocked entry is filtered; dial
	// proceeds to the public IP (which will fail to connect on port 1, but
	// with a dial error, NOT ErrBlockedTarget).
	restore := setLookupIPAddr(func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{
			{IP: net.ParseIP("10.0.0.1")},
			{IP: net.ParseIP("203.0.113.1")}, // TEST-NET-3, unroutable but public
		}, nil
	})
	defer restore()

	dial := SafeDialContext(false)
	_, err := dial(context.Background(), "tcp", "mixed.test:1")
	if errors.Is(err, ErrBlockedTarget) {
		t.Fatalf("mixed resolver should not produce ErrBlockedTarget, got %v", err)
	}
}
