package procnet

import (
	"testing"
	"time"
)

func TestProcInfoStruct(t *testing.T) {
	p := ProcInfo{
		PID:   1234,
		Name:  "chrome",
		Conns: 5,
	}
	if p.PID != 1234 {
		t.Fatalf("PID = %d, want 1234", p.PID)
	}
	if p.Name != "chrome" {
		t.Fatalf("Name = %s, want chrome", p.Name)
	}
	if p.Conns != 5 {
		t.Fatalf("Conns = %d, want 5", p.Conns)
	}
}

func TestNewResolver(t *testing.T) {
	r := NewResolver()
	if r == nil {
		t.Fatal("NewResolver returned nil")
	}
	if r.ttl != 5*time.Second {
		t.Fatalf("ttl = %v, want 5s", r.ttl)
	}
	if r.pidNames == nil {
		t.Fatal("pidNames is nil")
	}
}

func TestResolverResolveUnknownPort(t *testing.T) {
	r := NewResolver()
	// Port 0 should return empty string (no process)
	name := r.Resolve(0)
	if name != "" {
		t.Fatalf("expected empty string for port 0, got %q", name)
	}
}

func TestResolverCacheTTL(t *testing.T) {
	r := NewResolver()
	r.ttl = 10 * time.Millisecond

	// First call triggers refresh
	r.Resolve(12345)

	r.mu.Lock()
	first := r.lastLoad
	r.mu.Unlock()

	// Wait for TTL to expire
	time.Sleep(15 * time.Millisecond)

	// Second call should trigger another refresh
	r.Resolve(12345)

	r.mu.Lock()
	second := r.lastLoad
	r.mu.Unlock()

	if !second.After(first) {
		t.Fatal("expected cache refresh after TTL expiry")
	}
}

func TestListNetworkProcesses(t *testing.T) {
	procs, err := ListNetworkProcesses()
	if err != nil {
		t.Fatalf("ListNetworkProcesses: %v", err)
	}
	// Should return at least one process (the test runner itself has network connections)
	// But we don't fail if empty — depends on permissions
	for _, p := range procs {
		if p.PID == 0 {
			t.Error("PID should not be 0")
		}
		if p.Name == "" {
			t.Error("Name should not be empty")
		}
		if p.Conns <= 0 {
			t.Errorf("Conns = %d for PID %d, expected > 0", p.Conns, p.PID)
		}
	}
}

func TestPortToPID(t *testing.T) {
	// Port 0 should always return 0
	pid := PortToPID(0)
	if pid != 0 {
		t.Fatalf("PortToPID(0) = %d, want 0", pid)
	}

	// Port 65535 is unlikely to be in use
	pid = PortToPID(65535)
	// Don't fail if nonzero — just ensure no panic
	_ = pid
}
