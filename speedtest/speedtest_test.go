package speedtest

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", cfg.Timeout)
	}
	if cfg.Concurrency != 10 {
		t.Errorf("Concurrency = %d, want 10", cfg.Concurrency)
	}
}

func TestNewTester(t *testing.T) {
	// With nil config, should use defaults
	tester := NewTester(nil)
	if tester.cfg.Timeout != 5*time.Second {
		t.Error("NewTester(nil) should use default timeout")
	}

	// With custom config
	customCfg := &TestConfig{
		Timeout:     10 * time.Second,
		Concurrency: 5,
	}
	tester = NewTester(customCfg)
	if tester.cfg.Timeout != 10*time.Second {
		t.Error("NewTester should use provided timeout")
	}
	if tester.cfg.Concurrency != 5 {
		t.Error("NewTester should use provided concurrency")
	}
}

func TestSortByLatency(t *testing.T) {
	results := []TestResult{
		{ServerAddr: "slow.example.com", Available: true, Latency: 500 * time.Millisecond},
		{ServerAddr: "unavailable.example.com", Available: false, Error: "connection refused"},
		{ServerAddr: "fast.example.com", Available: true, Latency: 100 * time.Millisecond},
		{ServerAddr: "medium.example.com", Available: true, Latency: 250 * time.Millisecond},
		{ServerAddr: "also-unavailable.example.com", Available: false, Error: "timeout"},
	}

	SortByLatency(results)

	// Available servers should come first, sorted by latency
	if !results[0].Available || results[0].ServerAddr != "fast.example.com" {
		t.Errorf("results[0] = %v, want fast.example.com (available)", results[0])
	}
	if !results[1].Available || results[1].ServerAddr != "medium.example.com" {
		t.Errorf("results[1] = %v, want medium.example.com (available)", results[1])
	}
	if !results[2].Available || results[2].ServerAddr != "slow.example.com" {
		t.Errorf("results[2] = %v, want slow.example.com (available)", results[2])
	}

	// Unavailable servers should come last
	if results[3].Available || results[4].Available {
		t.Error("Unavailable servers should be at the end")
	}
}

func TestTestResult(t *testing.T) {
	result := TestResult{
		ServerAddr: "test.example.com:443",
		ServerName: "Test Server",
		Latency:    150 * time.Millisecond,
		LatencyMs:  150,
		Available:  true,
	}

	if result.ServerAddr != "test.example.com:443" {
		t.Errorf("ServerAddr = %q, want %q", result.ServerAddr, "test.example.com:443")
	}
	if result.ServerName != "Test Server" {
		t.Errorf("ServerName = %q, want %q", result.ServerName, "Test Server")
	}
	if result.Latency != 150*time.Millisecond {
		t.Errorf("Latency = %v, want 150ms", result.Latency)
	}
	if !result.Available {
		t.Error("Available = false, want true")
	}
}

func TestServer(t *testing.T) {
	srv := Server{
		Addr:     "proxy.example.com:443",
		Name:     "My Proxy",
		Password: "secret",
		SNI:      "www.example.com",
	}

	if srv.Addr != "proxy.example.com:443" {
		t.Errorf("Addr = %q, want %q", srv.Addr, "proxy.example.com:443")
	}
	if srv.Name != "My Proxy" {
		t.Errorf("Name = %q, want %q", srv.Name, "My Proxy")
	}
	if srv.SNI != "www.example.com" {
		t.Errorf("SNI = %q, want %q", srv.SNI, "www.example.com")
	}
}

// TestTestAllWithLocalServer tests the TestAll function with a local test server.
func TestTestAllWithLocalServer(t *testing.T) {
	// Start a local TCP server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer listener.Close()

	// Accept connections in background
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	tester := NewTester(&TestConfig{
		Timeout:     2 * time.Second,
		Concurrency: 5,
	})

	servers := []Server{
		{Addr: listener.Addr().String(), Name: "Local Test"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results := tester.TestAll(ctx, servers)

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}

	// Local server should be available
	if !results[0].Available {
		t.Errorf("Local server should be available, got error: %s", results[0].Error)
	}
}

// TestTestAllWithClosedPort tests that closed ports fail properly.
func TestTestAllWithClosedPort(t *testing.T) {
	// Start and immediately close a listener to get a "used" port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to get port: %v", err)
	}
	closedAddr := listener.Addr().String()
	listener.Close() // Close immediately - port should be closed

	tester := NewTester(&TestConfig{
		Timeout:     1 * time.Second,
		Concurrency: 5,
	})

	servers := []Server{
		{Addr: closedAddr, Name: "Closed Port"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	results := tester.TestAll(ctx, servers)

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}

	// Closed port should fail
	if results[0].Available {
		t.Error("Closed port should not be available")
	}
}

// TestTestAllStreamWithLocalServer tests the streaming test function.
func TestTestAllStreamWithLocalServer(t *testing.T) {
	// Start a local TCP server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer listener.Close()

	// Accept connections in background
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	tester := NewTester(&TestConfig{
		Timeout:     2 * time.Second,
		Concurrency: 5,
	})

	servers := []Server{
		{Addr: listener.Addr().String(), Name: "Local Test 1"},
		{Addr: listener.Addr().String(), Name: "Local Test 2"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resultCh := make(chan TestResult, len(servers))
	go tester.TestAllStream(ctx, servers, resultCh)

	results := make([]TestResult, 0, len(servers))
	for result := range resultCh {
		results = append(results, result)
	}

	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}

	for _, r := range results {
		if !r.Available {
			t.Errorf("Server %s should be available, got error: %s", r.ServerName, r.Error)
		}
	}
}

func TestContextCancellation(t *testing.T) {
	tester := NewTester(&TestConfig{
		Timeout:     10 * time.Second, // Long timeout
		Concurrency: 5,
	})

	servers := []Server{
		{Addr: "192.0.2.1:12345", Name: "Should be cancelled"}, // TEST-NET-1, should be unreachable
	}

	// Cancel immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results := tester.TestAll(ctx, servers)

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}

	// Should fail due to cancellation
	if results[0].Available {
		t.Error("Cancelled test should not be available")
	}
}

func TestEmptyServerList(t *testing.T) {
	tester := NewTester(nil)
	ctx := context.Background()

	results := tester.TestAll(ctx, []Server{})

	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
}
