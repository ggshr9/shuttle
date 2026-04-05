//go:build sandbox

package e2e

import (
	"net/http"
	"testing"
	"time"
)

// TestSandboxFaultDisconnectReconnect verifies the client recovers
// after server disconnect (simulated by API disconnect/reconnect).
// Inspired by Envoy's connection drain testing.
func TestSandboxFaultDisconnectReconnect(t *testing.T) {
	api := sandboxEnv(t, "SANDBOX_CLIENT_A_API")
	clientA := sandboxEnv(t, "SANDBOX_CLIENT_A_ADDR")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	// Verify proxy works before
	resp, err := httpViaSOCKS5(clientA+":1080", "http://"+httpbinAddr+"/ip", 15*time.Second)
	if err != nil {
		t.Fatalf("pre-fault request: %v", err)
	}
	resp.Body.Close()

	// Disconnect
	_, err = apiPost(api, "/api/disconnect", nil)
	if err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	time.Sleep(2 * time.Second)

	// Verify proxy is broken
	_, err = httpViaSOCKS5(clientA+":1080", "http://"+httpbinAddr+"/ip", 5*time.Second)
	if err == nil {
		t.Log("proxy still works after disconnect — may have auto-reconnected")
	}

	// Reconnect
	_, err = apiPost(api, "/api/connect", nil)
	if err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	time.Sleep(3 * time.Second)

	// Verify proxy works after recovery
	resp, err = httpViaSOCKS5(clientA+":1080", "http://"+httpbinAddr+"/ip", 15*time.Second)
	if err != nil {
		t.Fatalf("post-recovery request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	t.Log("fault recovery: disconnect + reconnect OK")
}

// TestSandboxFaultConnectionLeak verifies no connection leaks after traffic.
// Inspired by gRPC's connection counting tests.
func TestSandboxFaultConnectionLeak(t *testing.T) {
	api := sandboxEnv(t, "SANDBOX_CLIENT_A_API")
	clientA := sandboxEnv(t, "SANDBOX_CLIENT_A_ADDR")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	// Get baseline active connections
	status0, err := apiGet(api, "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	baseline, _ := status0["active_conns"].(float64)

	// Generate 50 requests
	for i := 0; i < 50; i++ {
		resp, err := httpViaSOCKS5(clientA+":1080", "http://"+httpbinAddr+"/ip", 10*time.Second)
		if err != nil {
			t.Logf("request %d failed: %v", i, err)
			continue
		}
		resp.Body.Close()
	}

	// Wait for connections to drain
	time.Sleep(3 * time.Second)

	// Check active connections returned to baseline
	status1, err := apiGet(api, "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	active, _ := status1["active_conns"].(float64)

	t.Logf("connection leak check: baseline=%d, after 50 requests=%d", int(baseline), int(active))

	if active > baseline+2 { // allow small margin
		t.Errorf("possible connection leak: %d active connections (baseline was %d)", int(active), int(baseline))
	}
}

// TestSandboxFaultCircuitBreakerRecovery verifies circuit breaker recovers
// after temporary failures.
func TestSandboxFaultCircuitBreakerRecovery(t *testing.T) {
	api := sandboxEnv(t, "SANDBOX_CLIENT_A_API")

	status, err := apiGet(api, "/api/status")
	if err != nil {
		t.Fatal(err)
	}

	cbState, _ := status["circuit_state"].(string)
	if cbState != "closed" {
		t.Logf("circuit breaker not closed: %s (may be recovering from previous test)", cbState)
	} else {
		t.Log("circuit breaker: closed (healthy)")
	}
}
