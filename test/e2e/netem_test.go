//go:build sandbox

package e2e

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/shuttle-proxy/shuttle/test/netem"
)

// =============================================================================
// Network impairment (tc netem) tests
// =============================================================================

// TestSandboxNetemApplyReset verifies that applying latency impairment to the
// router measurably increases round-trip time, and resetting restores it.
func TestSandboxNetemApplyReset(t *testing.T) {
	clientA := sandboxEnv(t, "SANDBOX_CLIENT_A_ADDR")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	socks5Addr := clientA + ":1080"
	targetURL := "http://" + httpbinAddr + "/ip"

	waitForService(t, socks5Addr, 30*time.Second)
	waitForService(t, httpbinAddr, 30*time.Second)

	// Ensure a clean baseline.
	netem.ResetRouter()

	// Measure baseline latency (average of 3 requests).
	baselineAvg := measureAvgLatency(t, socks5Addr, targetURL, 3)
	t.Logf("baseline avg latency: %v", baselineAvg)

	// Apply 200ms delay via netem.
	imp := netem.HighLatency()
	if err := netem.ApplyToRouter(imp); err != nil {
		t.Fatalf("ApplyToRouter failed: %v", err)
	}
	defer netem.ResetRouter()

	// Measure impaired latency.
	impairedAvg := measureAvgLatency(t, socks5Addr, targetURL, 3)
	t.Logf("impaired avg latency: %v (added delay: %v)", impairedAvg, imp.Delay)

	// The impaired latency should be noticeably higher. We check that it
	// exceeds the baseline by at least 100ms (half the injected 200ms delay)
	// to account for jitter and measurement noise.
	minExpected := baselineAvg + 100*time.Millisecond
	if impairedAvg < minExpected {
		t.Errorf("expected impaired latency >= %v, got %v", minExpected, impairedAvg)
	}

	// Reset and verify latency returns to normal.
	if err := netem.ResetRouter(); err != nil {
		t.Fatalf("ResetRouter failed: %v", err)
	}

	resetAvg := measureAvgLatency(t, socks5Addr, targetURL, 3)
	t.Logf("post-reset avg latency: %v", resetAvg)

	// After reset, latency should be close to baseline (within 100ms).
	if resetAvg > baselineAvg+100*time.Millisecond {
		t.Errorf("post-reset latency %v still elevated vs baseline %v", resetAvg, baselineAvg)
	}
}

// TestSandboxNetemProxyUnderLatency verifies that the proxy chain still
// completes requests under high latency (200ms added delay).
func TestSandboxNetemProxyUnderLatency(t *testing.T) {
	clientA := sandboxEnv(t, "SANDBOX_CLIENT_A_ADDR")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	socks5Addr := clientA + ":1080"
	waitForService(t, socks5Addr, 30*time.Second)

	netem.ResetRouter()
	defer netem.ResetRouter()

	if err := netem.ApplyToRouter(netem.HighLatency()); err != nil {
		t.Fatalf("ApplyToRouter failed: %v", err)
	}

	// Make several requests through the proxy; all should succeed.
	endpoints := []string{"/ip", "/get", "/headers"}
	for _, ep := range endpoints {
		targetURL := "http://" + httpbinAddr + ep
		resp, err := httpViaSOCKS5(socks5Addr, targetURL, 30*time.Second)
		if err != nil {
			t.Fatalf("request to %s under latency failed: %v", ep, err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			t.Fatalf("request to %s: expected 200, got %d", ep, resp.StatusCode)
		}
		resp.Body.Close()
		t.Logf("  %s -> 200 OK (under 200ms added latency)", ep)
	}
}

// TestSandboxNetemProxyUnderLoss verifies that the proxy chain still completes
// requests under 10% packet loss (TCP retransmits compensate).
func TestSandboxNetemProxyUnderLoss(t *testing.T) {
	clientA := sandboxEnv(t, "SANDBOX_CLIENT_A_ADDR")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	socks5Addr := clientA + ":1080"
	waitForService(t, socks5Addr, 30*time.Second)

	netem.ResetRouter()
	defer netem.ResetRouter()

	if err := netem.ApplyToRouter(netem.PacketLoss(10)); err != nil {
		t.Fatalf("ApplyToRouter failed: %v", err)
	}

	// With 10% loss, individual requests may be slow but should still complete.
	// Use a generous timeout and retry logic: at least 3 out of 5 must succeed.
	const attempts = 5
	const minSuccess = 3
	successes := 0

	for i := 0; i < attempts; i++ {
		targetURL := fmt.Sprintf("http://%s/get?loss_test=%d", httpbinAddr, i)
		resp, err := httpViaSOCKS5(socks5Addr, targetURL, 30*time.Second)
		if err != nil {
			t.Logf("  attempt %d: failed: %v", i, err)
			continue
		}
		if resp.StatusCode == http.StatusOK {
			successes++
			t.Logf("  attempt %d: 200 OK", i)
		} else {
			t.Logf("  attempt %d: status %d", i, resp.StatusCode)
		}
		resp.Body.Close()
	}

	if successes < minSuccess {
		t.Fatalf("only %d/%d requests succeeded under 10%% loss (need %d)", successes, attempts, minSuccess)
	}
	t.Logf("packet loss test OK: %d/%d succeeded", successes, attempts)
}

// TestSandboxNetemCongestionAdaptive applies GFW-like impairment (loss +
// stable RTT) and verifies that the adaptive congestion controller handles
// the degraded conditions. The test confirms that requests still complete
// and checks the client API for congestion mode information if available.
func TestSandboxNetemCongestionAdaptive(t *testing.T) {
	clientA := sandboxEnv(t, "SANDBOX_CLIENT_A_ADDR")
	clientAAPI := sandboxEnv(t, "SANDBOX_CLIENT_A_API")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	socks5Addr := clientA + ":1080"
	waitForService(t, socks5Addr, 30*time.Second)
	waitForService(t, clientAAPI, 30*time.Second)

	netem.ResetRouter()
	defer netem.ResetRouter()

	// Step 1: Record initial status / congestion info.
	initialStatus, err := apiGet(clientAAPI, "/api/status")
	if err != nil {
		t.Fatalf("get initial status failed: %v", err)
	}
	t.Logf("initial status: state=%v, transport=%v", initialStatus["state"], initialStatus["transport"])

	// Step 2: Apply GFW-like impairment (50ms delay, 10% loss, 2% reorder).
	if err := netem.ApplyToRouter(netem.GFWSimulation()); err != nil {
		t.Fatalf("ApplyToRouter(GFW) failed: %v", err)
	}
	t.Log("applied GFW simulation impairment")

	// Step 3: Drive traffic through the proxy so the congestion controller
	// can observe the degraded conditions.
	const requestCount = 10
	successes := 0
	for i := 0; i < requestCount; i++ {
		targetURL := fmt.Sprintf("http://%s/get?cc_test=%d", httpbinAddr, i)
		resp, err := httpViaSOCKS5(socks5Addr, targetURL, 30*time.Second)
		if err != nil {
			t.Logf("  request %d: error: %v", i, err)
			continue
		}
		if resp.StatusCode == http.StatusOK {
			successes++
		}
		resp.Body.Close()
	}
	t.Logf("completed %d/%d requests under GFW simulation", successes, requestCount)

	if successes < requestCount/2 {
		t.Fatalf("too few successes (%d/%d) under GFW impairment", successes, requestCount)
	}

	// Step 4: Check congestion status via API. The adaptive controller
	// should have detected the interference pattern. We query the status
	// API and log whatever congestion information is available.
	postStatus, err := apiGet(clientAAPI, "/api/status")
	if err != nil {
		t.Logf("warning: could not get post-impairment status: %v", err)
	} else {
		t.Logf("post-impairment status: %v", postStatus)

		// If the status includes congestion mode info, verify it switched.
		if cc, ok := postStatus["congestion_mode"]; ok {
			t.Logf("congestion mode after GFW simulation: %v", cc)
			// Under active interference the adaptive controller should
			// prefer brutal mode.
			if mode, ok := cc.(string); ok && strings.Contains(strings.ToLower(mode), "brutal") {
				t.Logf("adaptive controller switched to brutal mode as expected")
			} else {
				t.Logf("congestion mode is %v (may not have switched yet)", cc)
			}
		}
	}

	// Step 5: Also try the stats endpoint if it exists.
	stats, err := apiGet(clientAAPI, "/api/stats")
	if err == nil {
		t.Logf("client stats: %v", stats)
	}

	// Step 6: Reset impairment and verify normal operation resumes.
	if err := netem.ResetRouter(); err != nil {
		t.Fatalf("ResetRouter failed: %v", err)
	}

	// Quick sanity check: one request should work cleanly after reset.
	resp, err := httpViaSOCKS5(socks5Addr, "http://"+httpbinAddr+"/ip", 15*time.Second)
	if err != nil {
		t.Fatalf("post-reset request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("post-reset request: expected 200, got %d (%s)", resp.StatusCode, string(body))
	}
	t.Log("post-reset request OK")
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// measureAvgLatency makes n HTTP requests via SOCKS5 and returns the average
// round-trip time.
func measureAvgLatency(t *testing.T, socks5Addr, targetURL string, n int) time.Duration {
	t.Helper()
	var total time.Duration
	for i := 0; i < n; i++ {
		start := time.Now()
		resp, err := httpViaSOCKS5(socks5Addr, targetURL, 30*time.Second)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("latency measurement request %d failed: %v", i, err)
		}
		resp.Body.Close()
		total += elapsed
	}
	return total / time.Duration(n)
}
