package selector

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shuttle-proxy/shuttle/transport"
)

// --- Selector tests (scheduler tests already in scheduler_test.go, multipath in multipath_test.go) ---

func TestSelectorNew(t *testing.T) {
	transports := []transport.ClientTransport{
		&fakeTransport{typeName: "h3", conn: &fakeConn{}},
		&fakeTransport{typeName: "reality", conn: &fakeConn{}},
	}
	s := New(transports, nil, nil)
	if s.Type() != "selector" {
		t.Fatalf("Type = %s, want selector", s.Type())
	}
	probes := s.Probes()
	if len(probes) != 2 {
		t.Fatalf("expected 2 probes, got %d", len(probes))
	}
}

func TestSelectorDialFirstAvailable(t *testing.T) {
	transports := []transport.ClientTransport{
		&fakeTransport{typeName: "h3", conn: &fakeConn{}},
		&fakeTransport{typeName: "reality", conn: &fakeConn{}},
	}
	s := New(transports, &Config{Strategy: StrategyPriority}, nil)

	conn, err := s.Dial(context.Background(), "")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if conn == nil {
		t.Fatal("Dial returned nil")
	}
	if s.ActiveTransport() != "h3" {
		t.Fatalf("ActiveTransport = %s, want h3", s.ActiveTransport())
	}
}

func TestSelectorDialFallback(t *testing.T) {
	transports := []transport.ClientTransport{
		&fakeTransport{typeName: "h3", dialErr: errors.New("network error")},
		&fakeTransport{typeName: "reality", conn: &fakeConn{}},
	}
	s := New(transports, &Config{Strategy: StrategyPriority}, nil)

	conn, err := s.Dial(context.Background(), "")
	if err != nil {
		t.Fatalf("Dial should fallback: %v", err)
	}
	if conn == nil {
		t.Fatal("Dial returned nil")
	}
	if s.ActiveTransport() != "reality" {
		t.Fatalf("ActiveTransport = %s, want reality after fallback", s.ActiveTransport())
	}
}

func TestSelectorDialAllFail(t *testing.T) {
	transports := []transport.ClientTransport{
		&fakeTransport{typeName: "h3", dialErr: errors.New("fail")},
		&fakeTransport{typeName: "reality", dialErr: errors.New("fail")},
	}
	s := New(transports, &Config{Strategy: StrategyPriority}, nil)

	_, err := s.Dial(context.Background(), "")
	if err == nil {
		t.Fatal("expected error when all transports fail")
	}
}

func TestSelectorLatencyStrategy(t *testing.T) {
	h3 := &fakeTransport{typeName: "h3", conn: &fakeConn{}}
	reality := &fakeTransport{typeName: "reality", conn: &fakeConn{}}
	transports := []transport.ClientTransport{h3, reality}
	s := New(transports, &Config{Strategy: StrategyLatency}, nil)

	s.mu.Lock()
	s.probes["h3"].Latency = 100 * time.Millisecond
	s.probes["h3"].Available = true
	s.probes["reality"].Latency = 30 * time.Millisecond
	s.probes["reality"].Available = true
	s.mu.Unlock()

	s.maybeSwitch()

	if s.ActiveTransport() != "reality" {
		t.Fatalf("ActiveTransport = %s, want reality (lowest latency)", s.ActiveTransport())
	}
}

func TestSelectorClose(t *testing.T) {
	transports := []transport.ClientTransport{
		&fakeTransport{typeName: "h3", conn: &fakeConn{}},
	}
	s := New(transports, nil, nil)
	err := s.Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestSelectorActivePaths(t *testing.T) {
	s := New(nil, &Config{Strategy: StrategyPriority}, nil)
	paths := s.ActivePaths()
	if paths != nil {
		t.Fatal("expected nil ActivePaths when not in multipath mode")
	}
}

func TestSelectorActiveTransportNone(t *testing.T) {
	s := New(nil, &Config{Strategy: StrategyPriority}, nil)
	if s.ActiveTransport() != "none" {
		t.Fatalf("expected 'none', got %s", s.ActiveTransport())
	}
}

// --- Migrator tests ---

func TestMigratorMigrateAndCleanup(t *testing.T) {
	transports := []transport.ClientTransport{&fakeTransport{typeName: "h3", conn: &fakeConn{}}}
	s := New(transports, nil, nil)
	m := NewMigrator(s, nil)

	m.Migrate(context.Background(), &fakeTransport{typeName: "reality", conn: &fakeConn{}}, "")
	m.Migrate(context.Background(), &fakeTransport{typeName: "cdn", conn: &fakeConn{}}, "")

	m.mu.Lock()
	count := len(m.activeConns)
	m.mu.Unlock()
	if count != 2 {
		t.Fatalf("activeConns = %d, want 2", count)
	}

	m.Cleanup()

	m.mu.Lock()
	count = len(m.activeConns)
	m.mu.Unlock()
	if count != 1 {
		t.Fatalf("after Cleanup: activeConns = %d, want 1", count)
	}
}

func TestMigratorCleanupSingleConn(t *testing.T) {
	transports := []transport.ClientTransport{&fakeTransport{typeName: "h3", conn: &fakeConn{}}}
	s := New(transports, nil, nil)
	m := NewMigrator(s, nil)

	m.Migrate(context.Background(), &fakeTransport{typeName: "reality", conn: &fakeConn{}}, "")
	m.Cleanup()

	m.mu.Lock()
	count := len(m.activeConns)
	m.mu.Unlock()
	if count != 1 {
		t.Fatalf("single conn cleanup: activeConns = %d, want 1", count)
	}
}

func TestMigratorMigrateFail(t *testing.T) {
	transports := []transport.ClientTransport{&fakeTransport{typeName: "h3", conn: &fakeConn{}}}
	s := New(transports, nil, nil)
	m := NewMigrator(s, nil)

	_, err := m.Migrate(context.Background(), &fakeTransport{typeName: "bad", dialErr: errors.New("fail")}, "")
	if err == nil {
		t.Fatal("expected error on failed migration")
	}
}

// --- newScheduler tests ---

func TestNewSchedulerFactory(t *testing.T) {
	tests := []string{"min-latency", "load-balance", "weighted", "", "unknown"}
	for _, name := range tests {
		sched := newScheduler(name)
		if sched == nil {
			t.Fatalf("newScheduler(%q) returned nil", name)
		}
	}
}

// --- Strategy constants ---

func TestStrategyConstants(t *testing.T) {
	if StrategyAuto != "auto" {
		t.Fatal("StrategyAuto")
	}
	if StrategyPriority != "priority" {
		t.Fatal("StrategyPriority")
	}
	if StrategyLatency != "latency" {
		t.Fatal("StrategyLatency")
	}
	if StrategyMultipath != "multipath" {
		t.Fatal("StrategyMultipath")
	}
}

// --- Probe test ---

func TestProbeAvailableTransport(t *testing.T) {
	ft := &fakeTransport{typeName: "h3", conn: &fakeConn{}}
	result := Probe(context.Background(), ft)
	if !result.Available {
		t.Fatal("expected available transport")
	}
	if result.Loss != 0 {
		t.Fatalf("expected 0 loss, got %f", result.Loss)
	}
}

func TestProbeUnavailableTransport(t *testing.T) {
	ft := &fakeTransport{typeName: "h3", dialErr: errors.New("fail")}
	result := Probe(context.Background(), ft)
	if result.Available {
		t.Fatal("expected unavailable transport")
	}
	if result.Loss != 1.0 {
		t.Fatalf("expected 1.0 loss, got %f", result.Loss)
	}
}
