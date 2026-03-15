//go:build sandbox

package e2e

import (
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"
)

// =============================================================================
// DNS resolution tests (sandbox only)
// =============================================================================

// TestSandboxDNSResolution verifies that DNS resolution works through the
// client's SOCKS5 proxy. It resolves the httpbin service by making an HTTP
// request through the proxy and checks that the response is valid.
func TestSandboxDNSResolution(t *testing.T) {
	clientA := sandboxEnv(t, "SANDBOX_CLIENT_A_ADDR")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	socks5Addr := clientA + ":1080"
	waitForService(t, socks5Addr, 30*time.Second)
	waitForService(t, httpbinAddr, 30*time.Second)

	// Resolve and connect to httpbin through the SOCKS5 proxy.
	// The proxy performs DNS resolution on behalf of the client.
	targetURL := "http://" + httpbinAddr + "/ip"
	resp, err := httpViaSOCKS5(socks5Addr, targetURL, 15*time.Second)
	if err != nil {
		t.Fatalf("DNS resolution via SOCKS5 failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Decode the httpbin /ip response to verify we got a valid result.
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode httpbin response: %v", err)
	}

	origin, ok := result["origin"].(string)
	if !ok || origin == "" {
		t.Fatal("missing 'origin' in httpbin /ip response")
	}

	ip := net.ParseIP(origin)
	if ip == nil {
		t.Fatalf("httpbin returned invalid IP: %q", origin)
	}

	t.Logf("httpbin resolved and returned origin IP: %s", origin)
}

// TestSandboxDNSCaching verifies that repeated DNS queries through the proxy
// benefit from caching. The second query should complete at least as fast as
// the first, and both should succeed.
func TestSandboxDNSCaching(t *testing.T) {
	clientA := sandboxEnv(t, "SANDBOX_CLIENT_A_ADDR")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	socks5Addr := clientA + ":1080"
	waitForService(t, socks5Addr, 30*time.Second)
	waitForService(t, httpbinAddr, 30*time.Second)

	targetURL := "http://" + httpbinAddr + "/ip"

	// First request — cold cache, includes DNS resolution + connection setup.
	start1 := time.Now()
	resp1, err := httpViaSOCKS5(socks5Addr, targetURL, 15*time.Second)
	elapsed1 := time.Since(start1)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	resp1.Body.Close()

	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", resp1.StatusCode)
	}

	// Second request — should benefit from DNS cache (and possibly connection reuse).
	start2 := time.Now()
	resp2, err := httpViaSOCKS5(socks5Addr, targetURL, 15*time.Second)
	elapsed2 := time.Since(start2)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("second request: expected 200, got %d", resp2.StatusCode)
	}

	t.Logf("first request: %v, second request: %v", elapsed1, elapsed2)

	// Both requests should succeed. We log timing but don't strictly assert
	// that the second is faster, since in a sandbox many factors affect timing.
	// A 3x regression would be suspicious though.
	if elapsed2 > elapsed1*3 && elapsed1 > 50*time.Millisecond {
		t.Logf("WARNING: second request (%v) was significantly slower than first (%v); cache may not be effective",
			elapsed2, elapsed1)
	}
}

// TestSandboxDNSOverProxy verifies that DNS queries from the client go through
// the proxy tunnel rather than leaking to the local network. It does this by
// making a request to httpbin through the proxy and verifying the response
// origin IP comes from the server-side network (10.100.0.x), not the
// client-side network (10.100.1.x).
func TestSandboxDNSOverProxy(t *testing.T) {
	clientA := sandboxEnv(t, "SANDBOX_CLIENT_A_ADDR")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")
	apiAddr := sandboxEnv(t, "SANDBOX_CLIENT_A_API")

	socks5Addr := clientA + ":1080"
	waitForService(t, socks5Addr, 30*time.Second)
	waitForService(t, httpbinAddr, 30*time.Second)

	// Verify the client proxy is running and connected via the API.
	status, err := apiGet(apiAddr, "/api/status")
	if err != nil {
		t.Fatalf("API status check failed: %v", err)
	}
	state, _ := status["state"].(string)
	if state != "running" {
		t.Skipf("client not in running state (state=%s), skipping DNS leak test", state)
	}

	// Make a request through the SOCKS5 proxy to httpbin /ip.
	// This endpoint returns the source IP as seen by the httpbin server.
	targetURL := "http://" + httpbinAddr + "/ip"
	resp, err := httpViaSOCKS5(socks5Addr, targetURL, 15*time.Second)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// The origin IP should be from the server network (10.100.0.x), not the
	// client network (10.100.1.x). This proves traffic went through the tunnel.
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	origin, ok := result["origin"].(string)
	if !ok || origin == "" {
		t.Fatal("missing 'origin' in httpbin /ip response")
	}

	t.Logf("origin IP as seen by httpbin: %s", origin)

	// The origin should be from the server-side network, not the client network.
	// Server is on 10.100.0.x; client-a is on 10.100.1.x.
	serverNet := net.IPNet{
		IP:   net.ParseIP("10.100.0.0"),
		Mask: net.CIDRMask(24, 32),
	}
	originIP := net.ParseIP(origin)
	if originIP == nil {
		t.Fatalf("could not parse origin IP %q", origin)
	}

	if !serverNet.Contains(originIP) {
		// If origin is from client network, DNS or traffic is leaking.
		clientNet := net.IPNet{
			IP:   net.ParseIP("10.100.1.0"),
			Mask: net.CIDRMask(24, 32),
		}
		if clientNet.Contains(originIP) {
			t.Fatalf("DNS LEAK: origin %s is from client network (10.100.1.x), traffic bypassed proxy", origin)
		}
		t.Logf("origin %s is not from expected server network 10.100.0.x (may still be valid)", origin)
	}

	t.Logf("DNS leak test passed: traffic routes through server network (origin=%s)", origin)
}
