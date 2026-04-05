//go:build sandbox

package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

// TestSandboxSubscriptionList verifies the subscription API endpoints work.
func TestSandboxSubscriptionList(t *testing.T) {
	api := sandboxEnv(t, "SANDBOX_CLIENT_A_API")

	// List subscriptions — returns an array, not a map.
	resp, err := http.Get("http://" + api + "/api/subscriptions")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var list []any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	t.Logf("subscriptions: %d items", len(list))
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

	time.Sleep(500 * time.Millisecond)

	// List subscriptions — returns array
	resp, err := http.Get("http://" + api + "/api/subscriptions")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	t.Logf("subscriptions after add: %s", string(body))

	// Delete the subscription
	req, err := http.NewRequest(http.MethodDelete, "http://"+api+"/api/subscriptions/"+subID, nil)
	if err != nil {
		t.Fatalf("build DELETE: %v", err)
	}
	delResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE: expected 200, got %d", delResp.StatusCode)
	}

	t.Logf("subscription lifecycle: add + delete OK (id=%s)", subID)
}

// TestSandboxPrometheusMetrics verifies the Prometheus-style metrics are available
// via the status API (the /api/prometheus endpoint is only on the GUI backend,
// not the headless API used in sandbox).
func TestSandboxPrometheusMetrics(t *testing.T) {
	api := sandboxEnv(t, "SANDBOX_CLIENT_A_API")

	// Use /api/status instead — it returns the same metrics in JSON form.
	status, err := apiGet(api, "/api/status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	// Verify key metrics are present
	if _, ok := status["active_conns"]; !ok {
		t.Error("missing active_conns in status")
	}
	if _, ok := status["bytes_sent"]; !ok {
		t.Error("missing bytes_sent in status")
	}
	if _, ok := status["upload_speed"]; !ok {
		t.Error("missing upload_speed in status")
	}

	t.Logf("status metrics OK: state=%v, conns=%v", status["state"], status["active_conns"])
}
