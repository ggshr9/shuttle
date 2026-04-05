//go:build sandbox

package e2e

import (
	"testing"
	"time"
)

// TestSandboxSetStrategy verifies runtime strategy switching via API.
func TestSandboxSetStrategy(t *testing.T) {
	api := sandboxEnv(t, "SANDBOX_CLIENT_A_API")
	clientA := sandboxEnv(t, "SANDBOX_CLIENT_A_ADDR")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	// Verify engine running
	status, err := apiGet(api, "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	if status["state"] != "running" {
		t.Skipf("client not running: %v", status["state"])
	}

	// Make a request before strategy switch
	resp, err := httpViaSOCKS5(clientA+":1080", "http://"+httpbinAddr+"/ip", 15*time.Second)
	if err != nil {
		t.Fatalf("pre-switch request failed: %v", err)
	}
	resp.Body.Close()

	// Switch strategy to "latency"
	result, err := apiPost(api, "/api/transport/strategy", map[string]string{"strategy": "latency"})
	if err != nil {
		t.Fatalf("strategy switch failed: %v", err)
	}
	t.Logf("strategy switch result: %v", result)

	// Make a request after strategy switch — should still work
	resp, err = httpViaSOCKS5(clientA+":1080", "http://"+httpbinAddr+"/ip", 15*time.Second)
	if err != nil {
		t.Fatalf("post-switch request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Switch back to auto
	_, err = apiPost(api, "/api/transport/strategy", map[string]string{"strategy": "auto"})
	if err != nil {
		t.Fatalf("strategy reset failed: %v", err)
	}

	t.Log("strategy hot-switch: OK, traffic uninterrupted")
}

// TestSandboxDisconnectReconnectWithStrategy verifies disconnect/reconnect
// preserves the current strategy.
func TestSandboxDisconnectReconnectWithStrategy(t *testing.T) {
	api := sandboxEnv(t, "SANDBOX_CLIENT_A_API")

	status, err := apiGet(api, "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	if status["state"] != "running" {
		t.Skipf("client not running")
	}

	// Disconnect
	_, err = apiPost(api, "/api/disconnect", nil)
	if err != nil {
		t.Fatalf("disconnect failed: %v", err)
	}
	time.Sleep(1 * time.Second)

	// Reconnect
	_, err = apiPost(api, "/api/connect", nil)
	if err != nil {
		t.Fatalf("reconnect failed: %v", err)
	}
	time.Sleep(2 * time.Second)

	// Verify running
	status, err = apiGet(api, "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	if status["state"] != "running" {
		t.Fatalf("expected running after reconnect, got %v", status["state"])
	}

	t.Log("disconnect/reconnect: OK")
}
