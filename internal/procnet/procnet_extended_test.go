package procnet

import (
	"sync"
	"testing"
	"time"
)

// --- ProcInfo edge cases ---

func TestProcInfoZeroValue(t *testing.T) {
	var p ProcInfo
	if p.PID != 0 {
		t.Errorf("zero ProcInfo PID = %d, want 0", p.PID)
	}
	if p.Name != "" {
		t.Errorf("zero ProcInfo Name = %q, want empty", p.Name)
	}
	if p.Conns != 0 {
		t.Errorf("zero ProcInfo Conns = %d, want 0", p.Conns)
	}
}

func TestProcInfoLargePID(t *testing.T) {
	p := ProcInfo{PID: 4294967295, Name: "maxpid", Conns: 1}
	if p.PID != 4294967295 {
		t.Errorf("PID = %d, want max uint32", p.PID)
	}
}

func TestProcInfoEmptyName(t *testing.T) {
	p := ProcInfo{PID: 100, Name: "", Conns: 3}
	if p.Name != "" {
		t.Errorf("Name = %q, want empty", p.Name)
	}
}

// --- PortToPID edge cases ---

func TestPortToPIDZero(t *testing.T) {
	pid := PortToPID(0)
	if pid != 0 {
		t.Errorf("PortToPID(0) = %d, want 0", pid)
	}
}

func TestPortToPIDMaxPort(t *testing.T) {
	// Port 65535 is unlikely to be in use, but should not panic
	pid := PortToPID(65535)
	_ = pid // just verify no panic
}

func TestPortToPIDPort1(t *testing.T) {
	// Port 1 is unlikely to be in use in test environment
	pid := PortToPID(1)
	_ = pid // just verify no panic
}

func TestPortToPIDHighPorts(t *testing.T) {
	// Verify various high ports don't cause panics
	ports := []uint16{49152, 50000, 55555, 60000, 65000, 65534}
	for _, port := range ports {
		pid := PortToPID(port)
		_ = pid
	}
}

func TestPortToPIDCommonPorts(t *testing.T) {
	// Common service ports - just verify no panics
	ports := []uint16{22, 53, 80, 443, 3306, 5432, 6379, 8080, 8443, 9090}
	for _, port := range ports {
		pid := PortToPID(port)
		_ = pid
	}
}

// --- ListNetworkProcesses edge cases ---

func TestListNetworkProcessesNoPanic(t *testing.T) {
	// Should never panic regardless of system state
	procs, err := ListNetworkProcesses()
	if err != nil {
		t.Fatalf("ListNetworkProcesses error: %v", err)
	}

	// Verify no duplicates by PID
	seen := make(map[uint32]bool)
	for _, p := range procs {
		if seen[p.PID] {
			t.Errorf("duplicate PID %d in process list", p.PID)
		}
		seen[p.PID] = true
	}
}

func TestListNetworkProcessesFieldsValid(t *testing.T) {
	procs, err := ListNetworkProcesses()
	if err != nil {
		t.Fatalf("ListNetworkProcesses error: %v", err)
	}

	for _, p := range procs {
		if p.PID == 0 {
			t.Error("PID should not be 0 in process list")
		}
		if p.Name == "" {
			t.Errorf("Name should not be empty for PID %d", p.PID)
		}
		if p.Conns <= 0 {
			t.Errorf("Conns should be > 0 for PID %d (%s), got %d", p.PID, p.Name, p.Conns)
		}
	}
}

// --- Resolver concurrent access tests ---

func TestResolverConcurrentResolve(t *testing.T) {
	r := NewResolver()
	var wg sync.WaitGroup

	// Concurrent resolves should not race
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(port uint16) {
			defer wg.Done()
			_ = r.Resolve(port)
		}(uint16(i + 1000)) //nolint:gosec // G115: test loop index, always non-negative and small
	}
	wg.Wait()
}

func TestResolverConcurrentResolveWithExpiry(t *testing.T) {
	r := NewResolver()
	r.ttl = 1 * time.Millisecond // Very short TTL to force refreshes

	var wg sync.WaitGroup

	// Concurrent resolves with very short TTL forces many refreshes
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(port uint16) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				_ = r.Resolve(port)
				time.Sleep(time.Millisecond)
			}
		}(uint16(i + 2000)) //nolint:gosec // G115: test loop index, always non-negative and small
	}
	wg.Wait()
}

func TestResolverCacheRefreshUpdatesLastLoad(t *testing.T) {
	r := NewResolver()
	r.ttl = 1 * time.Millisecond

	// Force initial refresh
	_ = r.Resolve(12345)

	r.mu.Lock()
	t1 := r.lastLoad
	r.mu.Unlock()

	if t1.IsZero() {
		t.Fatal("lastLoad should be set after Resolve")
	}

	time.Sleep(5 * time.Millisecond)

	// Should trigger another refresh
	_ = r.Resolve(12345)

	r.mu.Lock()
	t2 := r.lastLoad
	r.mu.Unlock()

	if !t2.After(t1) {
		t.Fatal("lastLoad should be updated after TTL expiry")
	}
}

func TestResolverCacheNotExpired(t *testing.T) {
	r := NewResolver()
	r.ttl = 10 * time.Second

	// First call sets cache
	_ = r.Resolve(12345)

	r.mu.Lock()
	t1 := r.lastLoad
	r.mu.Unlock()

	// Immediately resolve again - should NOT refresh
	_ = r.Resolve(12345)

	r.mu.Lock()
	t2 := r.lastLoad
	r.mu.Unlock()

	if !t1.Equal(t2) {
		t.Fatal("lastLoad should not change within TTL")
	}
}

func TestResolverReturnsEmptyForUnusedPort(t *testing.T) {
	r := NewResolver()
	// Port 1 is unlikely to be in use
	name := r.Resolve(1)
	if name != "" {
		// This is OK if something actually uses port 1
		t.Logf("port 1 resolved to %q (unexpected but not wrong)", name)
	}
}

func TestResolverMultiplePorts(t *testing.T) {
	r := NewResolver()

	// Resolve multiple ports in succession
	ports := []uint16{0, 1, 80, 443, 8080, 12345, 65535}
	for _, port := range ports {
		name := r.Resolve(port)
		_ = name // just verify no panic
	}
}

// --- Resolver internal state tests ---

func TestResolverPidNamesInitialized(t *testing.T) {
	r := NewResolver()
	if r.pidNames == nil {
		t.Fatal("pidNames should be initialized")
	}
	if len(r.pidNames) != 0 {
		t.Fatalf("pidNames should be empty initially, got %d entries", len(r.pidNames))
	}
}

func TestResolverDefaultTTL(t *testing.T) {
	r := NewResolver()
	if r.ttl != 5*time.Second {
		t.Fatalf("default TTL = %v, want 5s", r.ttl)
	}
}

func TestResolverLastLoadInitiallyZero(t *testing.T) {
	r := NewResolver()
	if !r.lastLoad.IsZero() {
		t.Fatal("lastLoad should be zero initially")
	}
}

func TestResolverRefreshPopulatesPidNames(t *testing.T) {
	r := NewResolver()

	// Force a refresh by resolving something
	_ = r.Resolve(0)

	r.mu.Lock()
	defer r.mu.Unlock()

	// After refresh, lastLoad should be set
	if r.lastLoad.IsZero() {
		t.Fatal("lastLoad should be set after refresh")
	}

	// pidNames may or may not have entries depending on system state,
	// but the map itself should exist
	if r.pidNames == nil {
		t.Fatal("pidNames should not be nil after refresh")
	}
}
