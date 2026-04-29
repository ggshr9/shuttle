package cdn

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ggshr9/shuttle/transport"
)

func TestCDNServerName(t *testing.T) {
	s := NewServer(&ServerConfig{
		ListenAddr: ":8443",
		Password:   "test-password",
	}, nil)
	if got := s.Type(); got != "cdn" {
		t.Errorf("Type() = %q, want %q", got, "cdn")
	}
}

func TestCDNServerConfig(t *testing.T) {
	cfg := &ServerConfig{
		ListenAddr: ":9443",
		CertFile:   "/path/to/cert.pem",
		KeyFile:    "/path/to/key.pem",
		Password:   "secret",
		Path:       "/custom/path",
	}
	s := NewServer(cfg, nil)

	if s.config.ListenAddr != ":9443" {
		t.Errorf("ListenAddr = %q, want %q", s.config.ListenAddr, ":9443")
	}
	if s.config.CertFile != "/path/to/cert.pem" {
		t.Errorf("CertFile = %q, want %q", s.config.CertFile, "/path/to/cert.pem")
	}
	if s.config.KeyFile != "/path/to/key.pem" {
		t.Errorf("KeyFile = %q, want %q", s.config.KeyFile, "/path/to/key.pem")
	}
	if s.config.Password != "secret" {
		t.Errorf("Password = %q, want %q", s.config.Password, "secret")
	}
	if s.config.Path != "/custom/path" {
		t.Errorf("Path = %q, want %q", s.config.Path, "/custom/path")
	}
}

func TestCDNServerDefaultPath(t *testing.T) {
	s := NewServer(&ServerConfig{
		Password: "test",
	}, nil)
	if s.config.Path != "/cdn/stream" {
		t.Errorf("default Path = %q, want %q", s.config.Path, "/cdn/stream")
	}
}

func TestCDNServerConnChannel(t *testing.T) {
	s := NewServer(&ServerConfig{
		Password: "test",
	}, nil)
	if s.connCh == nil {
		t.Error("connCh should not be nil")
	}
	if cap(s.connCh) != 64 {
		t.Errorf("connCh capacity = %d, want 64", cap(s.connCh))
	}
}

func TestCDNServerCloseBeforeListen(t *testing.T) {
	s := NewServer(&ServerConfig{
		Password: "test",
	}, nil)
	// Closing before Listen should not panic
	if err := s.Close(); err != nil {
		t.Errorf("Close() before Listen() returned error: %v", err)
	}
}

// TestCDNServerHandshakeMetricsWired asserts the handshake metrics hook is
// stored on the server struct after SetHandshakeMetrics is called.
func TestCDNServerHandshakeMetricsWired(t *testing.T) {
	s := NewServer(&ServerConfig{Password: "test"}, nil)
	if s.metrics != nil {
		t.Fatalf("metrics should be nil before SetHandshakeMetrics")
	}
	hook := &transport.HandshakeMetrics{
		OnSuccess: func(string, time.Duration) {},
		OnFailure: func(string, string) {},
	}
	s.SetHandshakeMetrics(hook)
	if s.metrics != hook {
		t.Fatalf("metrics not stored after SetHandshakeMetrics")
	}
}

// TestCDNServerHandshakeHookFires_AuthFailure drives handleStream directly
// with a malformed (empty) auth payload — the read fails and OnFailure must
// fire with the "protocol" reason. This exercises the wiring end-to-end
// without needing real TLS infrastructure.
func TestCDNServerHandshakeHookFires_AuthFailure(t *testing.T) {
	var failures int32
	var lastTransport, lastReason string
	hook := &transport.HandshakeMetrics{
		OnFailure: func(tn, reason string) {
			atomic.AddInt32(&failures, 1)
			lastTransport, lastReason = tn, reason
		},
	}

	s := NewServer(&ServerConfig{Password: "secret"}, nil)
	s.SetHandshakeMetrics(hook)

	// Empty body — io.ReadFull will return io.EOF, classifyReason → "protocol".
	req := httptest.NewRequest(http.MethodPost, "/cdn/stream", bytes.NewReader(nil))
	rec := httptest.NewRecorder()
	s.handleStream(rec, req)

	if got := atomic.LoadInt32(&failures); got != 1 {
		t.Fatalf("expected 1 OnFailure call, got %d", got)
	}
	if lastTransport != "cdn" {
		t.Errorf("expected transport %q, got %q", "cdn", lastTransport)
	}
	if lastReason != "protocol" {
		t.Errorf("expected reason %q, got %q", "protocol", lastReason)
	}
}

// TestCDNServerHandshakeHookFires_BadMethod ensures non-POST requests count
// as protocol failures.
func TestCDNServerHandshakeHookFires_BadMethod(t *testing.T) {
	var failures int32
	hook := &transport.HandshakeMetrics{
		OnFailure: func(string, string) { atomic.AddInt32(&failures, 1) },
	}

	s := NewServer(&ServerConfig{Password: "secret"}, nil)
	s.SetHandshakeMetrics(hook)

	req := httptest.NewRequest(http.MethodGet, "/cdn/stream", nil)
	rec := httptest.NewRecorder()
	s.handleStream(rec, req)

	if got := atomic.LoadInt32(&failures); got != 1 {
		t.Fatalf("expected 1 OnFailure call, got %d", got)
	}
}
