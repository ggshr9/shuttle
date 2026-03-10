package admin

import (
	"bytes"
	"strings"
	"testing"
)

func TestWritePrometheusMetrics_Basic(t *testing.T) {
	info := &ServerInfo{}
	info.ActiveConns.Store(5)
	info.TotalConns.Store(100)
	info.BytesSent.Store(1024)
	info.BytesRecv.Store(2048)

	var buf bytes.Buffer
	WritePrometheusMetrics(&buf, info, nil)
	out := buf.String()

	expected := []string{
		"# HELP shuttle_active_connections Number of active connections",
		"# TYPE shuttle_active_connections gauge",
		"shuttle_active_connections 5",
		"# TYPE shuttle_total_connections counter",
		"shuttle_total_connections 100",
		"# TYPE shuttle_bytes_sent_total counter",
		"shuttle_bytes_sent_total 1024",
		"# TYPE shuttle_bytes_received_total counter",
		"shuttle_bytes_received_total 2048",
		"# TYPE shuttle_goroutines gauge",
		"# TYPE shuttle_memory_alloc_bytes gauge",
		"# TYPE shuttle_memory_sys_bytes gauge",
		"# TYPE shuttle_gc_total counter",
	}

	for _, line := range expected {
		if !strings.Contains(out, line) {
			t.Errorf("output missing expected line: %q", line)
		}
	}
}

func TestWritePrometheusMetrics_WithUsers(t *testing.T) {
	info := &ServerInfo{}
	users := NewUserStore(nil)
	u1, err := users.Add("alice", 0)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate traffic by looking up internal user and setting atomics
	users.mu.RLock()
	internal := users.users[u1.Token]
	users.mu.RUnlock()
	internal.BytesSent.Store(500)
	internal.BytesRecv.Store(300)
	internal.ActiveConns.Store(2)

	var buf bytes.Buffer
	WritePrometheusMetrics(&buf, info, users)
	out := buf.String()

	expected := []string{
		`shuttle_user_bytes_sent_total{user="alice"} 500`,
		`shuttle_user_bytes_received_total{user="alice"} 300`,
		`shuttle_user_active_connections{user="alice"} 2`,
	}

	for _, line := range expected {
		if !strings.Contains(out, line) {
			t.Errorf("output missing expected line: %q\nfull output:\n%s", line, out)
		}
	}
}

func TestWritePrometheusMetrics_NilUsers(t *testing.T) {
	info := &ServerInfo{}

	var buf bytes.Buffer
	// Should not panic with nil users
	WritePrometheusMetrics(&buf, info, nil)

	out := buf.String()
	if !strings.Contains(out, "shuttle_active_connections") {
		t.Error("expected server metrics even with nil users")
	}
	if strings.Contains(out, "shuttle_user_") {
		t.Error("expected no per-user metrics with nil users")
	}
}
