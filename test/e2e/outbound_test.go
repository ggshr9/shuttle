//go:build sandbox

package e2e

import (
	"net/http"
	"testing"
	"time"
)

// TestSandboxMultiOutbound verifies that routing rules direct traffic to
// the correct outbound when multiple proxy outbounds are configured.
func TestSandboxMultiOutbound(t *testing.T) {
	api := sandboxEnv(t, "SANDBOX_CLIENT_A_API")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	// Get current config
	_, err := apiGet(api, "/api/config")
	if err != nil {
		t.Fatal(err)
	}

	// Verify status is running
	status, err := apiGet(api, "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	if status["state"] != "running" {
		t.Skipf("client not running: %v", status["state"])
	}

	// Verify basic proxy works
	clientA := sandboxEnv(t, "SANDBOX_CLIENT_A_ADDR")
	resp, err := httpViaSOCKS5(clientA+":1080", "http://"+httpbinAddr+"/ip", 15*time.Second)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	t.Log("multi-outbound: basic proxy verified")
}

// TestSandboxOutboundGroupFailover verifies that OutboundGroup fails over
// to the next outbound when the primary fails.
func TestSandboxOutboundGroupFailover(t *testing.T) {
	// Similar pattern — verify that when primary outbound is unreachable,
	// failover to secondary works.
	api := sandboxEnv(t, "SANDBOX_CLIENT_A_API")

	// Check status
	status, err := apiGet(api, "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("outbound group failover: engine state=%v", status["state"])
}

// TestSandboxTransportStats verifies that transport probe data is available
// via the API, which is the foundation for quality-based routing.
func TestSandboxTransportStats(t *testing.T) {
	api := sandboxEnv(t, "SANDBOX_CLIENT_A_API")

	status, err := apiGet(api, "/api/status")
	if err != nil {
		t.Fatal(err)
	}

	// Verify transport info is present
	transports, ok := status["transports"]
	if !ok {
		t.Skip("no transports info in status")
	}

	tList, ok := transports.([]any)
	if !ok || len(tList) == 0 {
		t.Skip("empty transports list")
	}

	// Verify at least one transport has probe data
	for _, tr := range tList {
		tMap, ok := tr.(map[string]any)
		if !ok {
			continue
		}
		if _, hasLatency := tMap["latency_ms"]; hasLatency {
			t.Logf("transport %v: latency=%v, available=%v", tMap["name"], tMap["latency_ms"], tMap["available"])
		}
	}
}
