package p2p

import (
	"log/slog"
	"net"
	"os"
	"testing"
	"time"
)

func TestFallbackControllerGetDecision(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	fc := NewFallbackController(nil, logger)

	vip := net.IPv4(10, 7, 0, 2)

	// New peer should default to direct
	decision := fc.GetDecision(vip)
	if decision != DecisionDirect {
		t.Errorf("expected DecisionDirect for new peer, got %v", decision)
	}
}

func TestFallbackControllerRecordSuccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	fc := NewFallbackController(nil, logger)

	vip := net.IPv4(10, 7, 0, 2)

	// Record some failures first
	for i := 0; i < 5; i++ {
		fc.RecordFailure(vip)
	}

	// Should be using relay now
	decision := fc.GetDecision(vip)
	if decision != DecisionRelay {
		t.Errorf("expected DecisionRelay after failures, got %v", decision)
	}

	// Record a success
	fc.RecordSuccess(vip, 10*time.Millisecond)

	// Should switch back to direct
	decision = fc.GetDecision(vip)
	if decision != DecisionDirect {
		t.Errorf("expected DecisionDirect after success, got %v", decision)
	}
}

func TestFallbackControllerFailureThreshold(t *testing.T) {
	cfg := &FallbackConfig{
		FailureThreshold: 3,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	fc := NewFallbackController(cfg, logger)

	vip := net.IPv4(10, 7, 0, 2)

	// Record failures up to threshold
	for i := 0; i < 2; i++ {
		decision := fc.RecordFailure(vip)
		if decision != DecisionDirect {
			t.Errorf("iteration %d: expected DecisionDirect before threshold, got %v", i, decision)
		}
	}

	// Third failure should trigger relay
	decision := fc.RecordFailure(vip)
	if decision != DecisionRelay {
		t.Errorf("expected DecisionRelay at threshold, got %v", decision)
	}
}

func TestFallbackControllerShouldRetryDirect(t *testing.T) {
	cfg := &FallbackConfig{
		DirectRetryInterval: 100 * time.Millisecond,
		FailureThreshold:    1,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	fc := NewFallbackController(cfg, logger)

	vip := net.IPv4(10, 7, 0, 2)

	// Force into relay mode
	fc.RecordFailure(vip)

	// Should not retry immediately
	if fc.ShouldRetryDirect(vip) {
		t.Error("should not retry direct immediately after failure")
	}

	// Wait for retry interval
	time.Sleep(150 * time.Millisecond)

	// Should retry now
	if !fc.ShouldRetryDirect(vip) {
		t.Error("should retry direct after interval")
	}
}

func TestFallbackControllerGetPeerStats(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	fc := NewFallbackController(nil, logger)

	vip := net.IPv4(10, 7, 0, 2)

	// Record some activity
	fc.RecordSuccess(vip, 10*time.Millisecond)
	fc.RecordSuccess(vip, 20*time.Millisecond)
	fc.RecordFailure(vip)

	stats := fc.GetPeerStats(vip)
	if stats == nil {
		t.Fatal("expected non-nil stats")
		return
	}

	if stats.PacketsSent != 2 {
		t.Errorf("expected 2 packets sent, got %d", stats.PacketsSent)
	}
	if stats.PacketsLost != 1 {
		t.Errorf("expected 1 packet lost, got %d", stats.PacketsLost)
	}
	if stats.AvgLatency == 0 {
		t.Error("expected non-zero average latency")
	}
}

func TestFallbackControllerReset(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	fc := NewFallbackController(nil, logger)

	vip := net.IPv4(10, 7, 0, 2)

	// Add some state
	fc.RecordSuccess(vip, 10*time.Millisecond)

	// Reset
	fc.Reset(vip)

	// Should be like a new peer now
	stats := fc.GetPeerStats(vip)
	if stats != nil {
		t.Error("expected nil stats after reset")
	}
}

func TestFallbackControllerGetAllPeers(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	fc := NewFallbackController(nil, logger)

	vip1 := net.IPv4(10, 7, 0, 2)
	vip2 := net.IPv4(10, 7, 0, 3)

	fc.RecordSuccess(vip1, 10*time.Millisecond)
	fc.RecordSuccess(vip2, 10*time.Millisecond)

	peers := fc.GetAllPeers()
	if len(peers) != 2 {
		t.Errorf("expected 2 peers, got %d", len(peers))
	}
}

func TestFallbackControllerDirectRelayLists(t *testing.T) {
	cfg := &FallbackConfig{FailureThreshold: 1}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	fc := NewFallbackController(cfg, logger)

	vip1 := net.IPv4(10, 7, 0, 2) // Will use direct
	vip2 := net.IPv4(10, 7, 0, 3) // Will use relay

	fc.RecordSuccess(vip1, 10*time.Millisecond)
	fc.RecordFailure(vip2) // Force to relay

	directPeers := fc.GetDirectPeers()
	relayPeers := fc.GetRelayPeers()

	if len(directPeers) != 1 {
		t.Errorf("expected 1 direct peer, got %d", len(directPeers))
	}
	if len(relayPeers) != 1 {
		t.Errorf("expected 1 relay peer, got %d", len(relayPeers))
	}
}

func TestFallbackControllerResetAll(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	fc := NewFallbackController(nil, logger)

	fc.RecordSuccess(net.IPv4(10, 7, 0, 2), 10*time.Millisecond)
	fc.RecordSuccess(net.IPv4(10, 7, 0, 3), 10*time.Millisecond)

	fc.ResetAll()

	peers := fc.GetAllPeers()
	if len(peers) != 0 {
		t.Errorf("expected 0 peers after ResetAll, got %d", len(peers))
	}
}

func TestDefaultFallbackConfig(t *testing.T) {
	cfg := DefaultFallbackConfig()

	if cfg.HolePunchTimeout == 0 {
		t.Error("expected non-zero HolePunchTimeout")
	}
	if cfg.DirectRetryInterval == 0 {
		t.Error("expected non-zero DirectRetryInterval")
	}
	if cfg.FailureThreshold == 0 {
		t.Error("expected non-zero FailureThreshold")
	}
	if cfg.LossThreshold == 0 {
		t.Error("expected non-zero LossThreshold")
	}
}
