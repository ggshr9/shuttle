//go:build sandbox

package subscription

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestFetch_BlockedByDialTimeValidation verifies that even when Add() passes
// (because the URL contains a hostname, not a literal IP), the dial-time
// check rejects the connection when the hostname resolves to a private IP.
func TestFetch_BlockedLoopbackLiteral(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("vmess://..."))
	}))
	defer ts.Close()

	m := NewManager()
	// Add literal loopback URL — this is blocked at Add time.
	_, err := m.Add("test", ts.URL)
	if err == nil {
		t.Fatal("expected Add to reject loopback literal")
	}
	if !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetch_AllowPrivateNetworksBypass(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("vmess://dGVzdA=="))
	}))
	defer ts.Close()

	m := NewManager()
	m.SetAllowPrivateNetworks(true)
	sub, err := m.Add("test", ts.URL)
	if err != nil {
		t.Fatalf("Add with allow-private: %v", err)
	}
	if _, err := m.Refresh(context.Background(), sub.ID); err != nil {
		t.Fatalf("Refresh with allow-private: %v", err)
	}
}

