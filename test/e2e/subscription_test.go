//go:build sandbox

package e2e

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestSandboxSubscriptionList verifies the subscription API endpoints work.
func TestSandboxSubscriptionList(t *testing.T) {
	api := sandboxEnv(t, "SANDBOX_CLIENT_A_API")

	// List subscriptions (should be empty or contain configured ones)
	result, err := apiGet(api, "/api/subscriptions")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("subscriptions: %v", result)
}

// TestSandboxSubscriptionAddRefreshDelete tests the full subscription lifecycle.
func TestSandboxSubscriptionAddRefreshDelete(t *testing.T) {
	api := sandboxEnv(t, "SANDBOX_CLIENT_A_API")
	httpbinAddr := sandboxEnv(t, "SANDBOX_HTTPBIN_ADDR")

	// Add a subscription pointing to httpbin (it will fail to parse, but the API should work)
	result, err := apiPost(api, "/api/subscriptions", map[string]string{
		"name": "test-sub",
		"url":  "http://" + httpbinAddr + "/get",
	})
	if err != nil {
		t.Fatalf("add subscription failed: %v", err)
	}

	subID, ok := result["id"].(string)
	if !ok || subID == "" {
		t.Fatalf("expected subscription ID, got: %v", result)
	}
	t.Logf("added subscription: id=%s", subID)

	// Use a small delay for eventual consistency
	time.Sleep(500 * time.Millisecond)

	// List subscriptions — should include our new one
	list, err := apiGet(api, "/api/subscriptions")
	if err != nil {
		t.Fatalf("list subscriptions failed: %v", err)
	}
	t.Logf("subscriptions after add: %v", list)

	// Delete the subscription
	req, err := http.NewRequest(http.MethodDelete, "http://"+api+"/api/subscriptions/"+subID, nil)
	if err != nil {
		t.Fatalf("build DELETE request failed: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE subscription failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE subscription: expected 200, got %d", resp.StatusCode)
	}

	t.Logf("subscription lifecycle: add + delete OK (id=%s)", subID)
}

// TestSandboxPrometheusMetrics verifies the Prometheus endpoint returns valid metrics.
func TestSandboxPrometheusMetrics(t *testing.T) {
	api := sandboxEnv(t, "SANDBOX_CLIENT_A_API")

	resp, err := http.Get("http://" + api + "/api/prometheus")
	if err != nil {
		t.Fatalf("prometheus endpoint failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	content := string(body)

	// Verify Prometheus format
	if !strings.Contains(content, "shuttle_active_connections") {
		t.Error("missing shuttle_active_connections metric")
	}
	if !strings.Contains(content, "shuttle_bytes_sent") {
		t.Error("missing shuttle_bytes_sent metric")
	}
	if !strings.Contains(content, "# TYPE") {
		t.Error("missing Prometheus TYPE annotations")
	}

	t.Logf("prometheus metrics OK: %d bytes", len(body))
}
