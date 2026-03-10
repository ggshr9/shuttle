package cdn

import (
	"testing"
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
