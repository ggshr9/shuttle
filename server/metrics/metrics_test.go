package metrics

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCollectorConnOpenClose(t *testing.T) {
	c := NewCollector()

	if c.ActiveConns.Load() != 0 {
		t.Fatal("expected 0 active conns initially")
	}

	c.ConnOpened("h3")
	c.ConnOpened("h3")
	c.ConnOpened("reality")

	if got := c.ActiveConns.Load(); got != 3 {
		t.Errorf("ActiveConns = %d, want 3", got)
	}
	if got := c.TotalConns.Load(); got != 3 {
		t.Errorf("TotalConns = %d, want 3", got)
	}

	c.ConnClosed("h3", 1*time.Second)

	if got := c.ActiveConns.Load(); got != 2 {
		t.Errorf("ActiveConns after close = %d, want 2", got)
	}
	// TotalConns should not decrease
	if got := c.TotalConns.Load(); got != 3 {
		t.Errorf("TotalConns after close = %d, want 3", got)
	}
}

func TestCollectorBytes(t *testing.T) {
	c := NewCollector()

	c.RecordBytes(100, 200)
	c.RecordBytes(50, 75)

	if got := c.BytesIn.Load(); got != 150 {
		t.Errorf("BytesIn = %d, want 150", got)
	}
	if got := c.BytesOut.Load(); got != 275 {
		t.Errorf("BytesOut = %d, want 275", got)
	}
}

func TestCollectorDurationHistogram(t *testing.T) {
	c := NewCollector()

	// Record connections with various durations
	// 50ms -> bucket 0.1
	c.ConnOpened("h3")
	c.ConnClosed("h3", 50*time.Millisecond)
	// 500ms -> bucket 0.5
	c.ConnOpened("h3")
	c.ConnClosed("h3", 500*time.Millisecond)
	// 2s -> bucket 5
	c.ConnOpened("h3")
	c.ConnClosed("h3", 2*time.Second)
	// 45s -> bucket 60
	c.ConnOpened("h3")
	c.ConnClosed("h3", 45*time.Second)

	var buf bytes.Buffer
	c.writeMetrics(&buf)
	out := buf.String()

	// Check histogram buckets are present
	// le="0.1" should have 1 (50ms)
	if !strings.Contains(out, `shuttle_connection_duration_seconds_bucket{le="0.1"} 1`) {
		t.Errorf("expected bucket le=0.1 count 1\noutput:\n%s", out)
	}
	// le="0.5" should have 2 (50ms + 500ms)
	if !strings.Contains(out, `shuttle_connection_duration_seconds_bucket{le="0.5"} 2`) {
		t.Errorf("expected bucket le=0.5 count 2\noutput:\n%s", out)
	}
	// le="5.0" should have 3 (50ms + 500ms + 2s)
	if !strings.Contains(out, `shuttle_connection_duration_seconds_bucket{le="5.0"} 3`) {
		t.Errorf("expected bucket le=5.0 count 3\noutput:\n%s", out)
	}
	// le="60.0" should have 4 (all)
	if !strings.Contains(out, `shuttle_connection_duration_seconds_bucket{le="60.0"} 4`) {
		t.Errorf("expected bucket le=60.0 count 4\noutput:\n%s", out)
	}
	// +Inf should have 4
	if !strings.Contains(out, `shuttle_connection_duration_seconds_bucket{le="+Inf"} 4`) {
		t.Errorf("expected +Inf bucket count 4\noutput:\n%s", out)
	}
	// count should be 4
	if !strings.Contains(out, "shuttle_connection_duration_seconds_count 4") {
		t.Errorf("expected count 4\noutput:\n%s", out)
	}
}

func TestCollectorTransportBreakdown(t *testing.T) {
	c := NewCollector()

	c.ConnOpened("h3")
	c.ConnOpened("h3")
	c.ConnOpened("reality")
	c.ConnOpened("cdn")
	c.ConnClosed("h3", 1*time.Second)

	var buf bytes.Buffer
	c.writeMetrics(&buf)
	out := buf.String()

	// h3: 1 active (2 opened, 1 closed), 2 total
	if !strings.Contains(out, `shuttle_transport_active_connections{transport="h3"} 1`) {
		t.Errorf("expected h3 active=1\noutput:\n%s", out)
	}
	if !strings.Contains(out, `shuttle_transport_connections_total{transport="h3"} 2`) {
		t.Errorf("expected h3 total=2\noutput:\n%s", out)
	}

	// reality: 1 active, 1 total
	if !strings.Contains(out, `shuttle_transport_active_connections{transport="reality"} 1`) {
		t.Errorf("expected reality active=1\noutput:\n%s", out)
	}

	// cdn: 1 active, 1 total
	if !strings.Contains(out, `shuttle_transport_active_connections{transport="cdn"} 1`) {
		t.Errorf("expected cdn active=1\noutput:\n%s", out)
	}
}

func TestCollectorHandler(t *testing.T) {
	c := NewCollector()
	c.ConnOpened("h3")
	c.RecordBytes(1024, 2048)
	c.RecordAuthFailure()

	handler := c.Handler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "shuttle_active_connections 1") {
		t.Errorf("missing active_connections in response\n%s", body)
	}
	if !strings.Contains(body, "shuttle_bytes_received_total 1024") {
		t.Errorf("missing bytes_received_total in response\n%s", body)
	}
	if !strings.Contains(body, "shuttle_auth_failures_total 1") {
		t.Errorf("missing auth_failures_total in response\n%s", body)
	}
}

func TestCollectorPrometheusFormat(t *testing.T) {
	c := NewCollector()
	c.ConnOpened("h3")
	c.StreamOpened()

	var buf bytes.Buffer
	c.writeMetrics(&buf)
	out := buf.String()

	// Verify HELP and TYPE lines precede values for each metric
	checks := []struct {
		help     string
		typeLine string
		value    string
	}{
		{
			"# HELP shuttle_active_connections Current number of active connections",
			"# TYPE shuttle_active_connections gauge",
			"shuttle_active_connections 1",
		},
		{
			"# HELP shuttle_active_streams Current number of active streams",
			"# TYPE shuttle_active_streams gauge",
			"shuttle_active_streams 1",
		},
		{
			"# HELP shuttle_connections_total Total connections since start",
			"# TYPE shuttle_connections_total counter",
			"shuttle_connections_total 1",
		},
		{
			"# HELP shuttle_streams_total Total streams since start",
			"# TYPE shuttle_streams_total counter",
			"shuttle_streams_total 1",
		},
		{
			"# HELP shuttle_connection_duration_seconds Connection duration histogram",
			"# TYPE shuttle_connection_duration_seconds histogram",
			"shuttle_connection_duration_seconds_count 0",
		},
		{
			"# HELP shuttle_uptime_seconds Time since server start",
			"# TYPE shuttle_uptime_seconds gauge",
			"",
		},
		{
			"# HELP shuttle_go_goroutines Number of goroutines",
			"# TYPE shuttle_go_goroutines gauge",
			"",
		},
	}

	for _, check := range checks {
		if !strings.Contains(out, check.help) {
			t.Errorf("missing HELP line: %q", check.help)
		}
		if !strings.Contains(out, check.typeLine) {
			t.Errorf("missing TYPE line: %q", check.typeLine)
		}
		if check.value != "" && !strings.Contains(out, check.value) {
			t.Errorf("missing value line: %q\noutput:\n%s", check.value, out)
		}

		// Verify HELP comes before TYPE
		helpIdx := strings.Index(out, check.help)
		typeIdx := strings.Index(out, check.typeLine)
		if helpIdx > typeIdx {
			t.Errorf("HELP should come before TYPE for %s", check.help)
		}
	}
}

func TestCollectorStreams(t *testing.T) {
	c := NewCollector()

	c.StreamOpened()
	c.StreamOpened()
	c.StreamOpened()

	if got := c.ActiveStreams.Load(); got != 3 {
		t.Errorf("ActiveStreams = %d, want 3", got)
	}
	if got := c.TotalStreams.Load(); got != 3 {
		t.Errorf("TotalStreams = %d, want 3", got)
	}

	c.StreamClosed()

	if got := c.ActiveStreams.Load(); got != 2 {
		t.Errorf("ActiveStreams after close = %d, want 2", got)
	}
	if got := c.TotalStreams.Load(); got != 3 {
		t.Errorf("TotalStreams should not decrease, got %d", got)
	}
}

func TestCollectorAuthFailures(t *testing.T) {
	c := NewCollector()

	c.RecordAuthFailure()
	c.RecordAuthFailure()
	c.RecordAuthFailure()

	if got := c.AuthFailures.Load(); got != 3 {
		t.Errorf("AuthFailures = %d, want 3", got)
	}

	var buf bytes.Buffer
	c.writeMetrics(&buf)
	if !strings.Contains(buf.String(), "shuttle_auth_failures_total 3") {
		t.Error("expected auth_failures_total 3 in output")
	}
}

func TestCollectorConcurrent(t *testing.T) {
	c := NewCollector()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.ConnOpened("h3")
			c.RecordBytes(100, 200)
			c.StreamOpened()
			c.StreamClosed()
			c.ConnClosed("h3", time.Millisecond)
		}()
	}
	wg.Wait()

	if got := c.ActiveConns.Load(); got != 0 {
		t.Errorf("expected 0 active conns, got %d", got)
	}
	if got := c.TotalConns.Load(); got != 100 {
		t.Errorf("expected 100 total conns, got %d", got)
	}
	if got := c.ActiveStreams.Load(); got != 0 {
		t.Errorf("expected 0 active streams, got %d", got)
	}
	if got := c.TotalStreams.Load(); got != 100 {
		t.Errorf("expected 100 total streams, got %d", got)
	}
	if got := c.BytesIn.Load(); got != 10000 {
		t.Errorf("expected 10000 bytes in, got %d", got)
	}
	if got := c.BytesOut.Load(); got != 20000 {
		t.Errorf("expected 20000 bytes out, got %d", got)
	}
}

func TestCollector_HandshakeHistogram(t *testing.T) {
	c := NewCollector()
	c.RecordHandshake("h3", 50*time.Millisecond)
	c.RecordHandshake("h3", 250*time.Millisecond)

	rr := httptest.NewRecorder()
	c.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	body := rr.Body.String()

	if !strings.Contains(body, `shuttle_handshake_duration_seconds_count{transport="h3"} 2`) {
		t.Fatalf("missing handshake count, got:\n%s", body)
	}
}

func TestCollector_HandshakeFailure(t *testing.T) {
	c := NewCollector()
	c.RecordHandshakeFailure("reality", "auth")
	c.RecordHandshakeFailure("reality", "auth")
	c.RecordHandshakeFailure("reality", "timeout")

	rr := httptest.NewRecorder()
	c.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	body := rr.Body.String()

	if !strings.Contains(body, `shuttle_handshake_failures_total{transport="reality",reason="auth"} 2`) {
		t.Fatalf("missing auth failure line, got:\n%s", body)
	}
	if !strings.Contains(body, `shuttle_handshake_failures_total{transport="reality",reason="timeout"} 1`) {
		t.Fatalf("missing timeout failure line, got:\n%s", body)
	}
}

func TestFormatFloat(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0.1, "0.1"},
		{0.5, "0.5"},
		{1.0, "1.0"},
		{5.0, "5.0"},
		{60.0, "60.0"},
		{3600.0, "3600.0"},
		{0.123456, "0.123456"},
	}
	for _, tt := range tests {
		got := formatFloat(tt.in)
		if got != tt.want {
			t.Errorf("formatFloat(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
