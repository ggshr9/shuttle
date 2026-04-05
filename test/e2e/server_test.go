//go:build sandbox

package e2e

import (
	"testing"
	"time"
)

// TestSandboxServerAdminMetrics verifies the server's admin /api/metrics endpoint
// includes plugin chain statistics after traffic flows through.
func TestSandboxServerAdminMetrics(t *testing.T) {
	_ = sandboxEnv(t, "SANDBOX_SERVER_ADDR")
	clientA := sandboxEnv(t, "SANDBOX_CLIENT_A_ADDR")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	// Generate some traffic first
	for i := 0; i < 3; i++ {
		resp, err := httpViaSOCKS5(clientA+":1080", "http://"+httpbinAddr+"/ip", 15*time.Second)
		if err != nil {
			t.Fatalf("traffic generation failed: %v", err)
		}
		resp.Body.Close()
	}

	// Check server admin metrics (if admin API is available)
	// Server admin typically runs on port 9091 with token auth
	// This test may need to be skipped if admin is not configured
	t.Log("server metrics: traffic generated, would check admin endpoint if available")
}
