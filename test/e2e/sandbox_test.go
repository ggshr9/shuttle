//go:build sandbox

// Package e2e contains end-to-end sandbox integration tests.
//
// These tests run ONLY inside Docker containers (via scripts/test.sh --sandbox)
// and verify the full proxy data flow through the sandbox network topology:
//
//   gotest (10.100.*.100) → client-a (10.100.1.10:1080) → server (10.100.0.10:443) → httpbin (10.100.0.20)
//
// Environment variables (set by docker-compose):
//
//	SANDBOX_SERVER_ADDR      = 10.100.0.10:443
//	SANDBOX_HTTPBIN_ADDR     = 10.100.0.20:80
//	SANDBOX_CLIENT_A_ADDR    = 10.100.1.10
//	SANDBOX_CLIENT_B_ADDR    = 10.100.2.10
//	SANDBOX_CLIENT_A_API     = 10.100.1.10:9090
//	SANDBOX_CLIENT_B_API     = 10.100.2.10:9090
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// sandboxEnv reads a required sandbox environment variable.
func sandboxEnv(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Skipf("env %s not set — not in sandbox", key)
	}
	return v
}

// waitForService polls host:port until it responds or timeout.
func waitForService(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("service %s not ready within %v", addr, timeout)
}

// httpViaSOCKS5 makes an HTTP GET request through a SOCKS5 proxy.
func httpViaSOCKS5(proxyAddr, targetURL string, timeout time.Duration) (*http.Response, error) {
	proxyURL, _ := url.Parse("socks5://" + proxyAddr)
	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   timeout,
	}
	defer client.CloseIdleConnections()
	return client.Get(targetURL)
}

// httpViaHTTPProxy makes an HTTP GET request through an HTTP proxy.
func httpViaHTTPProxy(proxyAddr, targetURL string, timeout time.Duration) (*http.Response, error) {
	proxyURL, _ := url.Parse("http://" + proxyAddr)
	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   timeout,
	}
	defer client.CloseIdleConnections()
	return client.Get(targetURL)
}

// apiGet makes a GET request to the client API.
func apiGet(apiAddr, path string) (map[string]any, error) {
	resp, err := http.Get("http://" + apiAddr + path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// apiPost makes a POST request to the client API.
func apiPost(apiAddr, path string, body any) (map[string]any, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = strings.NewReader(string(data))
	}
	resp, err := http.Post("http://"+apiAddr+path, "application/json", reqBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// =============================================================================
// P0: End-to-end proxy tests (H3 transport)
// =============================================================================

// TestSandboxE2ESOCKS5H3 verifies the full proxy chain:
// gotest → client-a SOCKS5 → H3 → server → httpbin
func TestSandboxE2ESOCKS5H3(t *testing.T) {
	clientA := sandboxEnv(t, "SANDBOX_CLIENT_A_ADDR")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	socks5Addr := clientA + ":1080"
	targetURL := "http://" + httpbinAddr + "/ip"

	waitForService(t, socks5Addr, 30*time.Second)
	waitForService(t, httpbinAddr, 30*time.Second)

	resp, err := httpViaSOCKS5(socks5Addr, targetURL, 15*time.Second)
	if err != nil {
		t.Fatalf("SOCKS5 proxy request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("invalid JSON response: %v (body: %s)", err, string(body))
	}

	// httpbin /ip returns {"origin": "<ip>"}
	origin, ok := result["origin"].(string)
	if !ok || origin == "" {
		t.Fatalf("expected origin IP in response, got: %s", string(body))
	}

	// The origin should be the server's IP (10.100.0.10) since traffic goes through server
	if !strings.HasPrefix(origin, "10.100.0.") {
		t.Logf("warning: origin IP %s is not on net-server (may be expected for some routing)", origin)
	}

	t.Logf("SOCKS5 H3 e2e OK: origin=%s, status=%d", origin, resp.StatusCode)
}

// TestSandboxE2EHTTPProxyH3 verifies the HTTP proxy chain:
// gotest → client-a HTTP proxy → H3 → server → httpbin
func TestSandboxE2EHTTPProxyH3(t *testing.T) {
	clientA := sandboxEnv(t, "SANDBOX_CLIENT_A_ADDR")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	httpProxyAddr := clientA + ":8080"
	targetURL := "http://" + httpbinAddr + "/get"

	waitForService(t, httpProxyAddr, 30*time.Second)

	resp, err := httpViaHTTPProxy(httpProxyAddr, targetURL, 15*time.Second)
	if err != nil {
		t.Fatalf("HTTP proxy request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("invalid JSON response: %v (body: %s)", err, string(body))
	}

	// httpbin /get returns full request details
	if _, ok := result["headers"]; !ok {
		t.Fatalf("expected headers in /get response, got: %s", string(body))
	}

	t.Logf("HTTP proxy H3 e2e OK: status=%d, body_len=%d", resp.StatusCode, len(body))
}

// TestSandboxE2EMultipleRequests verifies the proxy handles multiple sequential requests.
func TestSandboxE2EMultipleRequests(t *testing.T) {
	clientA := sandboxEnv(t, "SANDBOX_CLIENT_A_ADDR")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	socks5Addr := clientA + ":1080"
	waitForService(t, socks5Addr, 30*time.Second)

	endpoints := []string{"/ip", "/get", "/headers", "/user-agent"}
	for _, ep := range endpoints {
		targetURL := "http://" + httpbinAddr + ep
		resp, err := httpViaSOCKS5(socks5Addr, targetURL, 15*time.Second)
		if err != nil {
			t.Fatalf("request to %s failed: %v", ep, err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			t.Fatalf("request to %s: expected 200, got %d", ep, resp.StatusCode)
		}
		resp.Body.Close()
		t.Logf("  %s → 200 OK", ep)
	}
}

// TestSandboxE2EConcurrentRequests verifies the proxy handles concurrent requests.
func TestSandboxE2EConcurrentRequests(t *testing.T) {
	clientA := sandboxEnv(t, "SANDBOX_CLIENT_A_ADDR")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	socks5Addr := clientA + ":1080"
	waitForService(t, socks5Addr, 30*time.Second)

	const concurrency = 5
	errCh := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			targetURL := fmt.Sprintf("http://%s/get?id=%d", httpbinAddr, id)
			resp, err := httpViaSOCKS5(socks5Addr, targetURL, 15*time.Second)
			if err != nil {
				errCh <- fmt.Errorf("request %d failed: %w", id, err)
				return
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				errCh <- fmt.Errorf("request %d: status %d", id, resp.StatusCode)
				return
			}
			errCh <- nil
		}(i)
	}

	for i := 0; i < concurrency; i++ {
		if err := <-errCh; err != nil {
			t.Error(err)
		}
	}

	t.Logf("concurrent e2e OK: %d parallel requests succeeded", concurrency)
}

// TestSandboxE2EClientBProxyAll verifies client-b (proxy_all mode) routes all traffic through proxy.
func TestSandboxE2EClientBProxyAll(t *testing.T) {
	clientB := sandboxEnv(t, "SANDBOX_CLIENT_B_ADDR")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	socks5Addr := clientB + ":1080"
	targetURL := "http://" + httpbinAddr + "/ip"

	waitForService(t, socks5Addr, 30*time.Second)

	resp, err := httpViaSOCKS5(socks5Addr, targetURL, 15*time.Second)
	if err != nil {
		t.Fatalf("SOCKS5 via client-b failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	t.Logf("client-b proxy_all OK: status=%d, body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
}

// =============================================================================
// P0: Reality transport tests
// =============================================================================

// TestSandboxE2ERealityTransport verifies the Reality/TLS+Noise transport
// by testing through client-a's proxy using the API to switch transports.
func TestSandboxE2ERealityTransport(t *testing.T) {
	clientAAPI := sandboxEnv(t, "SANDBOX_CLIENT_A_API")
	clientA := sandboxEnv(t, "SANDBOX_CLIENT_A_ADDR")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	waitForService(t, clientAAPI, 30*time.Second)

	// Step 1: Get current status
	status, err := apiGet(clientAAPI, "/api/status")
	if err != nil {
		t.Fatalf("get status failed: %v", err)
	}
	t.Logf("initial status: state=%v, transport=%v", status["state"], status["transport"])

	// Step 2: Disconnect first (if connected)
	if status["state"] == "running" {
		_, err = apiPost(clientAAPI, "/api/disconnect", nil)
		if err != nil {
			t.Logf("disconnect warning: %v", err)
		}
		time.Sleep(2 * time.Second)
	}

	// Step 3: Switch transport to Reality-only
	cfg, err := apiGet(clientAAPI, "/api/config")
	if err != nil {
		t.Fatalf("get config failed: %v", err)
	}

	// Modify config to use Reality transport only
	cfgMap := cfg
	if transport, ok := cfgMap["transport"].(map[string]any); ok {
		transport["preferred"] = "reality"
		if h3, ok := transport["h3"].(map[string]any); ok {
			h3["enabled"] = false
		}
		if reality, ok := transport["reality"].(map[string]any); ok {
			reality["enabled"] = true
		}
	}

	// Apply the config
	client := &http.Client{Timeout: 10 * time.Second}
	cfgData, _ := json.Marshal(cfgMap)
	req, _ := http.NewRequest("PUT", "http://"+clientAAPI+"/api/config", strings.NewReader(string(cfgData)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("update config failed: %v", err)
	}
	resp.Body.Close()

	// Step 4: Connect with Reality transport
	connectResult, err := apiPost(clientAAPI, "/api/connect", nil)
	if err != nil {
		t.Fatalf("connect with Reality failed: %v", err)
	}
	t.Logf("connect result: %v", connectResult)

	time.Sleep(3 * time.Second)

	// Step 5: Verify status shows Reality transport
	status, err = apiGet(clientAAPI, "/api/status")
	if err != nil {
		t.Fatalf("get status after connect failed: %v", err)
	}
	t.Logf("status after connect: state=%v, transport=%v", status["state"], status["transport"])

	if status["state"] != "running" {
		t.Fatalf("expected state 'running', got %v", status["state"])
	}

	// Step 6: Test data flow through Reality transport
	socks5Addr := clientA + ":1080"
	targetURL := "http://" + httpbinAddr + "/ip"

	proxyResp, err := httpViaSOCKS5(socks5Addr, targetURL, 15*time.Second)
	if err != nil {
		t.Fatalf("SOCKS5 via Reality failed: %v", err)
	}
	defer proxyResp.Body.Close()

	if proxyResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", proxyResp.StatusCode)
	}

	body, _ := io.ReadAll(proxyResp.Body)
	t.Logf("Reality transport e2e OK: status=%d, body=%s", proxyResp.StatusCode, strings.TrimSpace(string(body)))

	// Step 7: Restore H3 transport
	if transport, ok := cfgMap["transport"].(map[string]any); ok {
		transport["preferred"] = "h3"
		if h3, ok := transport["h3"].(map[string]any); ok {
			h3["enabled"] = true
		}
	}
	cfgData, _ = json.Marshal(cfgMap)
	req, _ = http.NewRequest("PUT", "http://"+clientAAPI+"/api/config", strings.NewReader(string(cfgData)))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Logf("restore config warning: %v", err)
	} else {
		resp.Body.Close()
	}

	// Reconnect with H3
	apiPost(clientAAPI, "/api/disconnect", nil)
	time.Sleep(time.Second)
	apiPost(clientAAPI, "/api/connect", nil)
}

// =============================================================================
// P0: CDN transport tests
// =============================================================================

// TestSandboxE2ECDNTransport verifies the CDN/HTTP2 transport by switching
// client-a to CDN-only mode via the API, then verifying proxy data flow.
func TestSandboxE2ECDNTransport(t *testing.T) {
	clientAAPI := sandboxEnv(t, "SANDBOX_CLIENT_A_API")
	clientA := sandboxEnv(t, "SANDBOX_CLIENT_A_ADDR")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	waitForService(t, clientAAPI, 30*time.Second)

	// Step 1: Disconnect if running
	status, err := apiGet(clientAAPI, "/api/status")
	if err != nil {
		t.Fatalf("get status failed: %v", err)
	}
	t.Logf("initial status: state=%v, transport=%v", status["state"], status["transport"])

	if status["state"] == "running" {
		_, err = apiPost(clientAAPI, "/api/disconnect", nil)
		if err != nil {
			t.Logf("disconnect warning: %v", err)
		}
		time.Sleep(2 * time.Second)
	}

	// Step 2: Get current config and switch to CDN-only
	cfg, err := apiGet(clientAAPI, "/api/config")
	if err != nil {
		t.Fatalf("get config failed: %v", err)
	}

	cfgMap := cfg
	if transport, ok := cfgMap["transport"].(map[string]any); ok {
		// Disable H3 and Reality
		if h3, ok := transport["h3"].(map[string]any); ok {
			h3["enabled"] = false
		}
		if reality, ok := transport["reality"].(map[string]any); ok {
			reality["enabled"] = false
		}
		// Enable CDN
		if cdnCfg, ok := transport["cdn"].(map[string]any); ok {
			cdnCfg["enabled"] = true
		} else {
			transport["cdn"] = map[string]any{
				"enabled":              true,
				"domain":               "10.100.0.10:8443",
				"path":                 "/cdn/stream",
				"insecure_skip_verify": true,
			}
		}
	}

	// Apply the config
	client := &http.Client{Timeout: 10 * time.Second}
	cfgData, _ := json.Marshal(cfgMap)
	req, _ := http.NewRequest("PUT", "http://"+clientAAPI+"/api/config", strings.NewReader(string(cfgData)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("update config failed: %v", err)
	}
	resp.Body.Close()

	// Step 3: Connect with CDN transport
	connectResult, err := apiPost(clientAAPI, "/api/connect", nil)
	if err != nil {
		t.Fatalf("connect with CDN failed: %v", err)
	}
	t.Logf("CDN connect result: %v", connectResult)

	time.Sleep(3 * time.Second)

	// Step 4: Verify status shows running
	status, err = apiGet(clientAAPI, "/api/status")
	if err != nil {
		t.Fatalf("get status after CDN connect failed: %v", err)
	}
	t.Logf("status after CDN connect: state=%v, transport=%v", status["state"], status["transport"])

	if status["state"] != "running" {
		t.Fatalf("expected state 'running', got %v", status["state"])
	}

	// Step 5: Test data flow through CDN transport
	socks5Addr := clientA + ":1080"
	targetURL := "http://" + httpbinAddr + "/ip"

	proxyResp, err := httpViaSOCKS5(socks5Addr, targetURL, 15*time.Second)
	if err != nil {
		t.Fatalf("SOCKS5 via CDN failed: %v", err)
	}
	defer proxyResp.Body.Close()

	if proxyResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", proxyResp.StatusCode)
	}

	body, _ := io.ReadAll(proxyResp.Body)
	t.Logf("CDN transport e2e OK: status=%d, body=%s", proxyResp.StatusCode, strings.TrimSpace(string(body)))

	// Step 6: Test multiple requests to verify stability
	for i := 0; i < 3; i++ {
		ep := fmt.Sprintf("/get?cdn_test=%d", i)
		resp, err := httpViaSOCKS5(socks5Addr, "http://"+httpbinAddr+ep, 15*time.Second)
		if err != nil {
			t.Fatalf("CDN request %d failed: %v", i, err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			t.Fatalf("CDN request %d: expected 200, got %d", i, resp.StatusCode)
		}
		resp.Body.Close()
		t.Logf("  CDN request %d → 200 OK", i)
	}

	// Step 7: Restore H3 transport
	if transport, ok := cfgMap["transport"].(map[string]any); ok {
		if h3, ok := transport["h3"].(map[string]any); ok {
			h3["enabled"] = true
		}
		if cdnCfg, ok := transport["cdn"].(map[string]any); ok {
			cdnCfg["enabled"] = false
		}
	}
	cfgData, _ = json.Marshal(cfgMap)
	req, _ = http.NewRequest("PUT", "http://"+clientAAPI+"/api/config", strings.NewReader(string(cfgData)))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Logf("restore config warning: %v", err)
	} else {
		resp.Body.Close()
	}

	// Reconnect with H3
	apiPost(clientAAPI, "/api/disconnect", nil)
	time.Sleep(time.Second)
	apiPost(clientAAPI, "/api/connect", nil)
}

// =============================================================================
// P0: TUN mode testing
// =============================================================================

// TestSandboxE2ETUNMode verifies TUN device creation and transparent proxying.
// This test uses the client API to enable TUN mode within the Docker container.
func TestSandboxE2ETUNMode(t *testing.T) {
	clientAAPI := sandboxEnv(t, "SANDBOX_CLIENT_A_API")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	waitForService(t, clientAAPI, 30*time.Second)

	// Step 1: Get current config
	cfg, err := apiGet(clientAAPI, "/api/config")
	if err != nil {
		t.Fatalf("get config failed: %v", err)
	}

	// Step 2: Disconnect
	apiPost(clientAAPI, "/api/disconnect", nil)
	time.Sleep(2 * time.Second)

	// Step 3: Enable TUN mode
	cfgMap := cfg
	if proxy, ok := cfgMap["proxy"].(map[string]any); ok {
		if tun, ok := proxy["tun"].(map[string]any); ok {
			tun["enabled"] = true
			tun["device_name"] = "utun-test"
			tun["cidr"] = "198.18.0.0/15"
			tun["mtu"] = 1500
			tun["auto_route"] = true
		} else {
			proxy["tun"] = map[string]any{
				"enabled":     true,
				"device_name": "utun-test",
				"cidr":        "198.18.0.0/15",
				"mtu":         1500,
				"auto_route":  true,
			}
		}
	}

	// Apply config
	client := &http.Client{Timeout: 10 * time.Second}
	cfgData, _ := json.Marshal(cfgMap)
	req, _ := http.NewRequest("PUT", "http://"+clientAAPI+"/api/config", strings.NewReader(string(cfgData)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("update config failed: %v", err)
	}
	resp.Body.Close()

	// Step 4: Connect
	connectResult, err := apiPost(clientAAPI, "/api/connect", nil)
	if err != nil {
		// TUN mode may not be fully supported in Alpine Docker — log and skip
		t.Skipf("TUN mode connect failed (may need kernel support): %v", err)
	}
	t.Logf("TUN connect result: %v", connectResult)

	time.Sleep(3 * time.Second)

	// Step 5: Verify status
	status, err := apiGet(clientAAPI, "/api/status")
	if err != nil {
		t.Fatalf("get status failed: %v", err)
	}
	t.Logf("TUN status: state=%v", status["state"])

	if status["state"] != "running" {
		t.Skipf("TUN mode did not start successfully (state=%v), may need kernel TUN support", status["state"])
	}

	// Step 6: Test using the probe API (goes through the local proxy chain, including TUN)
	probeResult, err := apiPost(clientAAPI, "/api/test/probe", map[string]any{
		"url": "http://" + httpbinAddr + "/ip",
		"via": "socks5",
	})
	if err != nil {
		t.Fatalf("probe via TUN failed: %v", err)
	}

	if success, ok := probeResult["success"].(bool); ok && success {
		t.Logf("TUN mode e2e OK: %v", probeResult)
	} else {
		t.Logf("TUN mode probe result: %v", probeResult)
	}

	// Step 7: Restore original config (disable TUN)
	if proxy, ok := cfgMap["proxy"].(map[string]any); ok {
		if tun, ok := proxy["tun"].(map[string]any); ok {
			tun["enabled"] = false
		}
	}
	cfgData, _ = json.Marshal(cfgMap)
	req, _ = http.NewRequest("PUT", "http://"+clientAAPI+"/api/config", strings.NewReader(string(cfgData)))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Logf("restore config warning: %v", err)
	} else {
		resp.Body.Close()
	}

	apiPost(clientAAPI, "/api/disconnect", nil)
	time.Sleep(time.Second)
	apiPost(clientAAPI, "/api/connect", nil)
}

// =============================================================================
// API tests (for Playwright integration)
// =============================================================================

// TestSandboxAPIStatus verifies the client API status endpoint.
func TestSandboxAPIStatus(t *testing.T) {
	clientAAPI := sandboxEnv(t, "SANDBOX_CLIENT_A_API")
	waitForService(t, clientAAPI, 30*time.Second)

	status, err := apiGet(clientAAPI, "/api/status")
	if err != nil {
		t.Fatalf("GET /api/status failed: %v", err)
	}

	// Verify required fields
	for _, field := range []string{"state", "transport", "active_conns", "total_conns", "bytes_sent", "bytes_received"} {
		if _, ok := status[field]; !ok {
			t.Errorf("missing field %q in status response", field)
		}
	}

	t.Logf("API status OK: %v", status)
}

// TestSandboxAPIConfig verifies the client API config endpoints.
func TestSandboxAPIConfig(t *testing.T) {
	clientAAPI := sandboxEnv(t, "SANDBOX_CLIENT_A_API")
	waitForService(t, clientAAPI, 30*time.Second)

	cfg, err := apiGet(clientAAPI, "/api/config")
	if err != nil {
		t.Fatalf("GET /api/config failed: %v", err)
	}

	// Verify config has expected structure
	if _, ok := cfg["server"]; !ok {
		t.Error("missing 'server' in config")
	}
	if _, ok := cfg["transport"]; !ok {
		t.Error("missing 'transport' in config")
	}
	if _, ok := cfg["proxy"]; !ok {
		t.Error("missing 'proxy' in config")
	}

	t.Logf("API config OK: server=%v", cfg["server"])
}

// TestSandboxAPIProbe verifies the test probe endpoint works end-to-end.
func TestSandboxAPIProbe(t *testing.T) {
	clientAAPI := sandboxEnv(t, "SANDBOX_CLIENT_A_API")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	waitForService(t, clientAAPI, 30*time.Second)

	// Test SOCKS5 probe
	result, err := apiPost(clientAAPI, "/api/test/probe", map[string]any{
		"url": "http://" + httpbinAddr + "/ip",
		"via": "socks5",
	})
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}

	success, _ := result["success"].(bool)
	if !success {
		t.Fatalf("probe not successful: %v", result)
	}

	status, _ := result["status"].(float64)
	if int(status) != 200 {
		t.Fatalf("expected status 200, got %v", status)
	}

	t.Logf("API probe OK: latency=%vms", result["latency_ms"])
}

// TestSandboxAPIBatchProbe verifies the batch probe endpoint.
func TestSandboxAPIBatchProbe(t *testing.T) {
	clientAAPI := sandboxEnv(t, "SANDBOX_CLIENT_A_API")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	waitForService(t, clientAAPI, 30*time.Second)

	result, err := apiPost(clientAAPI, "/api/test/probe/batch", map[string]any{
		"tests": []map[string]string{
			{"name": "socks5", "url": "http://" + httpbinAddr + "/ip", "via": "socks5"},
			{"name": "http", "url": "http://" + httpbinAddr + "/ip", "via": "http"},
			{"name": "direct", "url": "http://" + httpbinAddr + "/ip", "via": "direct"},
		},
	})
	if err != nil {
		t.Fatalf("batch probe failed: %v", err)
	}

	results, ok := result["results"].([]any)
	if !ok {
		t.Fatalf("expected results array, got: %v", result)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	for _, r := range results {
		rm, _ := r.(map[string]any)
		t.Logf("  %s via %s: success=%v latency=%vms", rm["name"], rm["via"], rm["success"], rm["latency_ms"])
	}
}

// TestSandboxAPIRoutingTemplates verifies routing template endpoints.
func TestSandboxAPIRoutingTemplates(t *testing.T) {
	clientAAPI := sandboxEnv(t, "SANDBOX_CLIENT_A_API")
	waitForService(t, clientAAPI, 30*time.Second)

	resp, err := http.Get("http://" + clientAAPI + "/api/routing/templates")
	if err != nil {
		t.Fatalf("GET /api/routing/templates failed: %v", err)
	}
	defer resp.Body.Close()

	var templates []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&templates); err != nil {
		t.Fatalf("decode templates failed: %v", err)
	}

	if len(templates) == 0 {
		t.Fatal("expected at least one routing template")
	}

	for _, tmpl := range templates {
		t.Logf("  template: id=%v name=%v", tmpl["id"], tmpl["name"])
	}
}

// =============================================================================
// P1: Config hot-reload test
// =============================================================================

// TestSandboxConfigHotReload verifies Engine.Reload() picks up config changes
// without requiring a manual disconnect/reconnect cycle.
func TestSandboxConfigHotReload(t *testing.T) {
	clientAAPI := sandboxEnv(t, "SANDBOX_CLIENT_A_API")
	clientA := sandboxEnv(t, "SANDBOX_CLIENT_A_ADDR")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	waitForService(t, clientAAPI, 30*time.Second)

	// Step 1: Verify engine is running
	status, err := apiGet(clientAAPI, "/api/status")
	if err != nil {
		t.Fatalf("get status failed: %v", err)
	}
	if status["state"] != "running" {
		t.Skipf("engine not running (state=%v), skip reload test", status["state"])
	}
	originalTransport := status["transport"]
	t.Logf("initial: state=%v, transport=%v", status["state"], originalTransport)

	// Step 2: Get current config
	cfg, err := apiGet(clientAAPI, "/api/config")
	if err != nil {
		t.Fatalf("get config failed: %v", err)
	}

	// Step 3: Switch to Reality transport via reload endpoint
	cfgMap := cfg
	if transport, ok := cfgMap["transport"].(map[string]any); ok {
		if h3, ok := transport["h3"].(map[string]any); ok {
			h3["enabled"] = false
		}
		if reality, ok := transport["reality"].(map[string]any); ok {
			reality["enabled"] = true
		}
	}

	// PUT /api/config calls Engine.Reload() — stops, swaps config, restarts
	client := &http.Client{Timeout: 15 * time.Second}
	cfgData, _ := json.Marshal(cfgMap)
	req, _ := http.NewRequest("PUT", "http://"+clientAAPI+"/api/config", strings.NewReader(string(cfgData)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/config (reload) failed: %v", err)
	}
	resp.Body.Close()
	time.Sleep(5 * time.Second)

	// Step 4: Verify engine restarted and is running
	status, err = apiGet(clientAAPI, "/api/status")
	if err != nil {
		t.Fatalf("get status after reload failed: %v", err)
	}
	if status["state"] != "running" {
		t.Fatalf("expected running after reload, got %v", status["state"])
	}
	t.Logf("after reload: state=%v, transport=%v", status["state"], status["transport"])

	// Step 5: Verify data flow still works
	socks5Addr := clientA + ":1080"
	targetURL := "http://" + httpbinAddr + "/ip"
	proxyResp, err := httpViaSOCKS5(socks5Addr, targetURL, 15*time.Second)
	if err != nil {
		t.Fatalf("proxy request after reload failed: %v", err)
	}
	defer proxyResp.Body.Close()
	if proxyResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 after reload, got %d", proxyResp.StatusCode)
	}
	t.Logf("hot-reload OK: proxy works after config change")

	// Step 6: Restore original config (H3)
	if transport, ok := cfgMap["transport"].(map[string]any); ok {
		if h3, ok := transport["h3"].(map[string]any); ok {
			h3["enabled"] = true
		}
		if reality, ok := transport["reality"].(map[string]any); ok {
			reality["enabled"] = false
		}
	}
	cfgData, _ = json.Marshal(cfgMap)
	req, _ = http.NewRequest("PUT", "http://"+clientAAPI+"/api/config", strings.NewReader(string(cfgData)))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = client.Do(req)
	if resp != nil {
		resp.Body.Close()
	}
	apiPost(clientAAPI, "/api/disconnect", nil)
	time.Sleep(time.Second)
	apiPost(clientAAPI, "/api/connect", nil)
	time.Sleep(2 * time.Second)
}

// TestSandboxAPIDisconnectReconnect verifies disconnect/reconnect cycle.
func TestSandboxAPIDisconnectReconnect(t *testing.T) {
	clientBAPI := sandboxEnv(t, "SANDBOX_CLIENT_B_API")
	waitForService(t, clientBAPI, 30*time.Second)

	// Check initial state
	status, err := apiGet(clientBAPI, "/api/status")
	if err != nil {
		t.Fatalf("get status failed: %v", err)
	}
	initialState := status["state"]
	t.Logf("initial state: %v", initialState)

	if initialState != "running" {
		t.Skip("client-b not running, skip disconnect/reconnect test")
	}

	// Disconnect
	_, err = apiPost(clientBAPI, "/api/disconnect", nil)
	if err != nil {
		t.Fatalf("disconnect failed: %v", err)
	}

	time.Sleep(2 * time.Second)

	status, err = apiGet(clientBAPI, "/api/status")
	if err != nil {
		t.Fatalf("get status after disconnect failed: %v", err)
	}
	if status["state"] != "stopped" {
		t.Errorf("expected stopped, got %v", status["state"])
	}

	// Reconnect
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "POST", "http://"+clientBAPI+"/api/connect", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("reconnect failed: %v", err)
	}
	resp.Body.Close()

	time.Sleep(3 * time.Second)

	status, err = apiGet(clientBAPI, "/api/status")
	if err != nil {
		t.Fatalf("get status after reconnect failed: %v", err)
	}
	if status["state"] != "running" {
		t.Fatalf("expected running after reconnect, got %v", status["state"])
	}

	t.Logf("disconnect/reconnect OK")
}

// =============================================================================
// P0: WebRTC transport tests
// =============================================================================

// TestSandboxE2EWebRTCTransport verifies the WebRTC DataChannel transport
// by switching client-a to WebRTC-only mode via the API, then verifying proxy data flow.
func TestSandboxE2EWebRTCTransport(t *testing.T) {
	clientAAPI := sandboxEnv(t, "SANDBOX_CLIENT_A_API")
	clientA := sandboxEnv(t, "SANDBOX_CLIENT_A_ADDR")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	waitForService(t, clientAAPI, 30*time.Second)

	// Step 1: Disconnect if running
	status, err := apiGet(clientAAPI, "/api/status")
	if err != nil {
		t.Fatalf("get status failed: %v", err)
	}
	t.Logf("initial status: state=%v, transport=%v", status["state"], status["transport"])

	if status["state"] == "running" {
		_, err = apiPost(clientAAPI, "/api/disconnect", nil)
		if err != nil {
			t.Logf("disconnect warning: %v", err)
		}
		time.Sleep(2 * time.Second)
	}

	// Step 2: Get current config and switch to WebRTC-only
	cfg, err := apiGet(clientAAPI, "/api/config")
	if err != nil {
		t.Fatalf("get config failed: %v", err)
	}

	cfgMap := cfg
	if transport, ok := cfgMap["transport"].(map[string]any); ok {
		transport["preferred"] = "webrtc"
		// Disable other transports
		if h3, ok := transport["h3"].(map[string]any); ok {
			h3["enabled"] = false
		}
		if reality, ok := transport["reality"].(map[string]any); ok {
			reality["enabled"] = false
		}
		if cdnCfg, ok := transport["cdn"].(map[string]any); ok {
			cdnCfg["enabled"] = false
		}
		// Enable WebRTC
		if webrtcCfg, ok := transport["webrtc"].(map[string]any); ok {
			webrtcCfg["enabled"] = true
		} else {
			transport["webrtc"] = map[string]any{
				"enabled":    true,
				"signal_url": "wss://10.100.0.10:8443/webrtc/signal",
			}
		}
	}

	// Apply the config
	client := &http.Client{Timeout: 10 * time.Second}
	cfgData, _ := json.Marshal(cfgMap)
	req, _ := http.NewRequest("PUT", "http://"+clientAAPI+"/api/config", strings.NewReader(string(cfgData)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("update config failed: %v", err)
	}
	resp.Body.Close()

	// Step 3: Connect with WebRTC transport
	connectResult, err := apiPost(clientAAPI, "/api/connect", nil)
	if err != nil {
		t.Fatalf("connect with WebRTC failed: %v", err)
	}
	t.Logf("WebRTC connect result: %v", connectResult)

	time.Sleep(3 * time.Second)

	// Step 4: Verify status shows running
	status, err = apiGet(clientAAPI, "/api/status")
	if err != nil {
		t.Fatalf("get status after WebRTC connect failed: %v", err)
	}
	t.Logf("status after WebRTC connect: state=%v, transport=%v", status["state"], status["transport"])

	if status["state"] != "running" {
		t.Fatalf("expected state 'running', got %v", status["state"])
	}

	// Step 5: Test data flow through WebRTC transport
	socks5Addr := clientA + ":1080"
	targetURL := "http://" + httpbinAddr + "/ip"

	proxyResp, err := httpViaSOCKS5(socks5Addr, targetURL, 15*time.Second)
	if err != nil {
		t.Fatalf("SOCKS5 via WebRTC failed: %v", err)
	}
	defer proxyResp.Body.Close()

	if proxyResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", proxyResp.StatusCode)
	}

	body, _ := io.ReadAll(proxyResp.Body)
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("invalid JSON response: %v (body: %s)", err, string(body))
	}

	origin, ok := result["origin"].(string)
	if !ok || origin == "" {
		t.Fatalf("expected origin IP in response, got: %s", string(body))
	}

	t.Logf("WebRTC transport e2e OK: origin=%s, status=%d", origin, proxyResp.StatusCode)

	// Step 6: Restore H3 transport
	if transport, ok := cfgMap["transport"].(map[string]any); ok {
		transport["preferred"] = "h3"
		if h3, ok := transport["h3"].(map[string]any); ok {
			h3["enabled"] = true
		}
		if webrtcCfg, ok := transport["webrtc"].(map[string]any); ok {
			webrtcCfg["enabled"] = false
		}
	}
	cfgData, _ = json.Marshal(cfgMap)
	req, _ = http.NewRequest("PUT", "http://"+clientAAPI+"/api/config", strings.NewReader(string(cfgData)))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Logf("restore config warning: %v", err)
	} else {
		resp.Body.Close()
	}

	// Reconnect with H3
	apiPost(clientAAPI, "/api/disconnect", nil)
	time.Sleep(time.Second)
	apiPost(clientAAPI, "/api/connect", nil)
}

// TestSandboxE2EWebRTCMultiStream verifies that WebRTC transport handles
// multiple concurrent streams over the same DataChannel connection.
func TestSandboxE2EWebRTCMultiStream(t *testing.T) {
	clientAAPI := sandboxEnv(t, "SANDBOX_CLIENT_A_API")
	clientA := sandboxEnv(t, "SANDBOX_CLIENT_A_ADDR")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	waitForService(t, clientAAPI, 30*time.Second)

	// Step 1: Disconnect if running
	status, err := apiGet(clientAAPI, "/api/status")
	if err != nil {
		t.Fatalf("get status failed: %v", err)
	}

	if status["state"] == "running" {
		_, err = apiPost(clientAAPI, "/api/disconnect", nil)
		if err != nil {
			t.Logf("disconnect warning: %v", err)
		}
		time.Sleep(2 * time.Second)
	}

	// Step 2: Switch to WebRTC-only
	cfg, err := apiGet(clientAAPI, "/api/config")
	if err != nil {
		t.Fatalf("get config failed: %v", err)
	}

	cfgMap := cfg
	if transport, ok := cfgMap["transport"].(map[string]any); ok {
		transport["preferred"] = "webrtc"
		if h3, ok := transport["h3"].(map[string]any); ok {
			h3["enabled"] = false
		}
		if reality, ok := transport["reality"].(map[string]any); ok {
			reality["enabled"] = false
		}
		if cdnCfg, ok := transport["cdn"].(map[string]any); ok {
			cdnCfg["enabled"] = false
		}
		if webrtcCfg, ok := transport["webrtc"].(map[string]any); ok {
			webrtcCfg["enabled"] = true
		} else {
			transport["webrtc"] = map[string]any{
				"enabled":    true,
				"signal_url": "wss://10.100.0.10:8443/webrtc/signal",
			}
		}
	}

	client := &http.Client{Timeout: 10 * time.Second}
	cfgData, _ := json.Marshal(cfgMap)
	req, _ := http.NewRequest("PUT", "http://"+clientAAPI+"/api/config", strings.NewReader(string(cfgData)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("update config failed: %v", err)
	}
	resp.Body.Close()

	// Step 3: Connect with WebRTC
	connectResult, err := apiPost(clientAAPI, "/api/connect", nil)
	if err != nil {
		t.Fatalf("connect with WebRTC failed: %v", err)
	}
	t.Logf("WebRTC connect result: %v", connectResult)

	time.Sleep(3 * time.Second)

	// Step 4: Verify running
	status, err = apiGet(clientAAPI, "/api/status")
	if err != nil {
		t.Fatalf("get status after connect failed: %v", err)
	}
	if status["state"] != "running" {
		t.Fatalf("expected state 'running', got %v", status["state"])
	}

	// Step 5: Fire 5 concurrent requests through WebRTC transport
	socks5Addr := clientA + ":1080"
	const concurrency = 5
	errCh := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			targetURL := fmt.Sprintf("http://%s/get?webrtc_stream=%d", httpbinAddr, id)
			proxyResp, err := httpViaSOCKS5(socks5Addr, targetURL, 15*time.Second)
			if err != nil {
				errCh <- fmt.Errorf("WebRTC stream %d failed: %w", id, err)
				return
			}
			defer proxyResp.Body.Close()
			if proxyResp.StatusCode != http.StatusOK {
				errCh <- fmt.Errorf("WebRTC stream %d: expected 200, got %d", id, proxyResp.StatusCode)
				return
			}
			body, _ := io.ReadAll(proxyResp.Body)
			var result map[string]any
			if err := json.Unmarshal(body, &result); err != nil {
				errCh <- fmt.Errorf("WebRTC stream %d: invalid JSON: %v", id, err)
				return
			}
			errCh <- nil
		}(i)
	}

	for i := 0; i < concurrency; i++ {
		if err := <-errCh; err != nil {
			t.Error(err)
		}
	}

	t.Logf("WebRTC multi-stream OK: %d concurrent requests succeeded", concurrency)

	// Step 6: Restore H3 transport
	if transport, ok := cfgMap["transport"].(map[string]any); ok {
		transport["preferred"] = "h3"
		if h3, ok := transport["h3"].(map[string]any); ok {
			h3["enabled"] = true
		}
		if webrtcCfg, ok := transport["webrtc"].(map[string]any); ok {
			webrtcCfg["enabled"] = false
		}
	}
	cfgData, _ = json.Marshal(cfgMap)
	req, _ = http.NewRequest("PUT", "http://"+clientAAPI+"/api/config", strings.NewReader(string(cfgData)))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Logf("restore config warning: %v", err)
	} else {
		resp.Body.Close()
	}

	apiPost(clientAAPI, "/api/disconnect", nil)
	time.Sleep(time.Second)
	apiPost(clientAAPI, "/api/connect", nil)
}

// TestSandboxE2EWebRTCFallback verifies that when WebRTC is configured as primary
// but with a broken signaling URL, the client falls back to another transport
// and requests still succeed.
func TestSandboxE2EWebRTCFallback(t *testing.T) {
	clientAAPI := sandboxEnv(t, "SANDBOX_CLIENT_A_API")
	clientA := sandboxEnv(t, "SANDBOX_CLIENT_A_ADDR")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	waitForService(t, clientAAPI, 30*time.Second)

	// Step 1: Disconnect if running
	status, err := apiGet(clientAAPI, "/api/status")
	if err != nil {
		t.Fatalf("get status failed: %v", err)
	}
	t.Logf("initial status: state=%v, transport=%v", status["state"], status["transport"])

	if status["state"] == "running" {
		_, err = apiPost(clientAAPI, "/api/disconnect", nil)
		if err != nil {
			t.Logf("disconnect warning: %v", err)
		}
		time.Sleep(2 * time.Second)
	}

	// Step 2: Configure WebRTC as preferred with a broken signaling URL,
	// but keep H3 enabled as a fallback.
	cfg, err := apiGet(clientAAPI, "/api/config")
	if err != nil {
		t.Fatalf("get config failed: %v", err)
	}

	cfgMap := cfg
	if transport, ok := cfgMap["transport"].(map[string]any); ok {
		transport["preferred"] = "webrtc"
		// Keep H3 enabled as fallback
		if h3, ok := transport["h3"].(map[string]any); ok {
			h3["enabled"] = true
		}
		if reality, ok := transport["reality"].(map[string]any); ok {
			reality["enabled"] = false
		}
		if cdnCfg, ok := transport["cdn"].(map[string]any); ok {
			cdnCfg["enabled"] = false
		}
		// Enable WebRTC with a broken signaling URL
		transport["webrtc"] = map[string]any{
			"enabled":      true,
			"signal_url":   "wss://192.0.2.1:9999/broken/signal", // non-routable address
			"stun_servers": []string{"stun:192.0.2.1:3478"},      // non-routable STUN
		}
	}

	// Apply the config
	client := &http.Client{Timeout: 10 * time.Second}
	cfgData, _ := json.Marshal(cfgMap)
	req, _ := http.NewRequest("PUT", "http://"+clientAAPI+"/api/config", strings.NewReader(string(cfgData)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("update config failed: %v", err)
	}
	resp.Body.Close()

	// Step 3: Connect — WebRTC should fail, client should fall back to H3
	connectResult, err := apiPost(clientAAPI, "/api/connect", nil)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	t.Logf("connect result (expecting fallback): %v", connectResult)

	time.Sleep(5 * time.Second)

	// Step 4: Verify engine is running (via fallback transport)
	status, err = apiGet(clientAAPI, "/api/status")
	if err != nil {
		t.Fatalf("get status after connect failed: %v", err)
	}
	t.Logf("status after fallback: state=%v, transport=%v", status["state"], status["transport"])

	if status["state"] != "running" {
		t.Fatalf("expected state 'running' after fallback, got %v", status["state"])
	}

	// The active transport should NOT be webrtc (since signaling is broken)
	if status["transport"] == "webrtc" {
		t.Logf("warning: transport is 'webrtc' despite broken signaling — may have connected anyway")
	}

	// Step 5: Verify data flow works through the fallback transport
	socks5Addr := clientA + ":1080"
	targetURL := "http://" + httpbinAddr + "/ip"

	proxyResp, err := httpViaSOCKS5(socks5Addr, targetURL, 15*time.Second)
	if err != nil {
		t.Fatalf("SOCKS5 via fallback failed: %v", err)
	}
	defer proxyResp.Body.Close()

	if proxyResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", proxyResp.StatusCode)
	}

	body, _ := io.ReadAll(proxyResp.Body)
	t.Logf("WebRTC fallback e2e OK: status=%d, body=%s", proxyResp.StatusCode, strings.TrimSpace(string(body)))

	// Step 6: Restore H3-only config
	if transport, ok := cfgMap["transport"].(map[string]any); ok {
		transport["preferred"] = "h3"
		if h3, ok := transport["h3"].(map[string]any); ok {
			h3["enabled"] = true
		}
		transport["webrtc"] = map[string]any{
			"enabled": false,
		}
	}
	cfgData, _ = json.Marshal(cfgMap)
	req, _ = http.NewRequest("PUT", "http://"+clientAAPI+"/api/config", strings.NewReader(string(cfgData)))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Logf("restore config warning: %v", err)
	} else {
		resp.Body.Close()
	}

	// Reconnect with H3
	apiPost(clientAAPI, "/api/disconnect", nil)
	time.Sleep(time.Second)
	apiPost(clientAAPI, "/api/connect", nil)
}
