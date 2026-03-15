package congestion

import (
	"testing"
	"time"
)

func TestNewAdaptiveDefaults(t *testing.T) {
	ac := NewAdaptive(nil, nil)
	if ac == nil {
		t.Fatal("NewAdaptive returned nil")
	}
	if ac.ActiveName() != "bbr" {
		t.Fatalf("initial active = %s, want bbr", ac.ActiveName())
	}
	if ac.lossThreshold != 0.05 {
		t.Fatalf("lossThreshold = %f, want 0.05", ac.lossThreshold)
	}
	if ac.switchCooldown != 10*time.Second {
		t.Fatalf("switchCooldown = %v, want 10s", ac.switchCooldown)
	}
}

func TestNewAdaptiveCustomConfig(t *testing.T) {
	ac := NewAdaptive(&AdaptiveConfig{
		BrutalRate:     50 * 1024 * 1024,
		LossThreshold:  0.10,
		SwitchCooldown: 5 * time.Second,
	}, nil)
	if ac.lossThreshold != 0.10 {
		t.Fatalf("lossThreshold = %f, want 0.10", ac.lossThreshold)
	}
	if ac.switchCooldown != 5*time.Second {
		t.Fatalf("switchCooldown = %v, want 5s", ac.switchCooldown)
	}
}

func TestAdaptiveStartsWithBBR(t *testing.T) {
	ac := NewAdaptive(nil, nil)
	if ac.ActiveName() != "bbr" {
		t.Fatalf("should start with bbr, got %s", ac.ActiveName())
	}
	// GetCwnd and GetPacingRate should work without panicking
	_ = ac.GetCwnd()
	_ = ac.GetPacingRate()
}

func TestAdaptiveOnPacketSent(t *testing.T) {
	ac := NewAdaptive(nil, nil)
	ac.OnPacketSent(1200)
	ac.OnPacketSent(1200)

	ac.mu.Lock()
	sent := ac.windowSentBytes
	ac.mu.Unlock()

	if sent != 2400 {
		t.Fatalf("windowSentBytes = %d, want 2400", sent)
	}
}

func TestAdaptiveOnAckRecordsRTT(t *testing.T) {
	ac := NewAdaptive(nil, nil)
	ac.OnAck(1200, 50*time.Millisecond)

	ac.mu.Lock()
	n := len(ac.rttHistory)
	ac.mu.Unlock()

	if n != 1 {
		t.Fatalf("rttHistory len = %d, want 1", n)
	}
}

func TestAdaptiveRTTHistoryCap(t *testing.T) {
	ac := NewAdaptive(nil, nil)
	for i := 0; i < 150; i++ {
		ac.OnAck(100, time.Duration(i)*time.Millisecond)
	}

	ac.mu.Lock()
	n := len(ac.rttHistory)
	ac.mu.Unlock()

	if n > 100 {
		t.Fatalf("rttHistory should be capped at 100, got %d", n)
	}
}

func TestAdaptiveSwitchToBrutalOnInterference(t *testing.T) {
	ac := NewAdaptive(&AdaptiveConfig{
		LossThreshold:  0.05,
		SwitchCooldown: 0,
		BrutalRate:     10 * 1024 * 1024,
	}, nil)

	// Build up RTT history (stable RTT → rttTrend <= 0)
	for i := 0; i < 30; i++ {
		ac.OnAck(1000, 50*time.Millisecond)
	}

	// Simulate high loss with stable RTT (interference pattern)
	ac.OnPacketSent(100000)
	for i := 0; i < 20; i++ {
		ac.OnPacketLoss(2000)
	}

	if ac.ActiveName() != "brutal" {
		t.Fatalf("expected switch to brutal on interference, got %s", ac.ActiveName())
	}
}

func TestAdaptiveStaysBBROnCongestion(t *testing.T) {
	ac := NewAdaptive(&AdaptiveConfig{
		LossThreshold:  0.05,
		SwitchCooldown: 0,
		BrutalRate:     10 * 1024 * 1024,
	}, nil)

	// Build up rising RTT history (congestion pattern)
	for i := 0; i < 30; i++ {
		rtt := time.Duration(50+i*5) * time.Millisecond
		ac.OnAck(1000, rtt)
	}

	// Simulate loss with rising RTT
	ac.OnPacketSent(100000)
	for i := 0; i < 20; i++ {
		ac.OnPacketLoss(2000)
	}

	if ac.ActiveName() != "bbr" {
		t.Fatalf("expected bbr on real congestion, got %s", ac.ActiveName())
	}
}

func TestAdaptiveSwitchCooldown(t *testing.T) {
	ac := NewAdaptive(&AdaptiveConfig{
		LossThreshold:  0.05,
		SwitchCooldown: 1 * time.Hour,
		BrutalRate:     10 * 1024 * 1024,
	}, nil)

	ac.mu.Lock()
	ac.switchTo(ac.brutal, "test")
	ac.mu.Unlock()

	if ac.ActiveName() != "brutal" {
		t.Fatal("should be brutal after forced switch")
	}

	ac.mu.Lock()
	ac.lossRate = 0
	ac.evaluateSwitch()
	ac.mu.Unlock()

	if ac.ActiveName() != "brutal" {
		t.Fatal("cooldown should prevent switch back to bbr")
	}
}

func TestAdaptiveRecoverToEfficientBBR(t *testing.T) {
	ac := NewAdaptive(&AdaptiveConfig{
		LossThreshold:  0.05,
		SwitchCooldown: 0,
		BrutalRate:     10 * 1024 * 1024,
	}, nil)

	ac.mu.Lock()
	ac.switchTo(ac.brutal, "test")
	// Reset lastSwitch so cooldown doesn't block
	ac.lastSwitch = time.Time{}
	ac.mu.Unlock()

	ac.mu.Lock()
	ac.lossRate = 0.01 // below threshold/2 = 0.025
	ac.evaluateSwitch()
	ac.mu.Unlock()

	if ac.ActiveName() != "bbr" {
		t.Fatalf("should recover to bbr on low loss, got %s", ac.ActiveName())
	}
}

func TestAdaptiveStats(t *testing.T) {
	ac := NewAdaptive(nil, nil)
	ac.OnPacketSent(5000)
	ac.OnAck(3000, 50*time.Millisecond)

	stats := ac.Stats()
	if stats["active"] != "bbr" {
		t.Fatalf("active = %v, want bbr", stats["active"])
	}
	for _, key := range []string{"lossRate", "rttTrend", "switchCount", "bbr", "brutal"} {
		if _, ok := stats[key]; !ok {
			t.Fatalf("missing %s in stats", key)
		}
	}
}

func TestAdaptiveRTTTrendCalculation(t *testing.T) {
	ac := NewAdaptive(nil, nil)

	for i := 0; i < 10; i++ {
		ac.OnAck(100, 50*time.Millisecond)
	}
	ac.mu.Lock()
	trend := ac.rttTrend
	ac.mu.Unlock()
	if trend != 0 {
		t.Fatalf("trend with <20 samples should be 0, got %f", trend)
	}

	for i := 0; i < 20; i++ {
		rtt := time.Duration(100+i*10) * time.Millisecond
		ac.OnAck(100, rtt)
	}
	ac.mu.Lock()
	trend = ac.rttTrend
	ac.mu.Unlock()
	if trend <= 0 {
		t.Fatalf("trend should be positive with rising RTT, got %f", trend)
	}
}

func TestAdaptiveLossWindowReset(t *testing.T) {
	ac := NewAdaptive(nil, nil)

	ac.mu.Lock()
	ac.lastWindowReset = time.Now().Add(-20 * time.Second)
	ac.windowSentBytes = 10000
	ac.windowLostBytes = 5000
	ac.resetWindowIfNeeded()
	sent := ac.windowSentBytes
	lost := ac.windowLostBytes
	ac.mu.Unlock()

	if sent != 0 || lost != 0 {
		t.Fatalf("window should be reset: sent=%d, lost=%d", sent, lost)
	}
}

func TestAdaptiveImplementsCongestionController(t *testing.T) {
	var _ CongestionController = (*AdaptiveCongestion)(nil)
}

func BenchmarkAdaptiveOnAck(b *testing.B) {
	ac := NewAdaptive(nil, nil)
	rtt := 50 * time.Millisecond
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ac.OnAck(1200, rtt)
	}
}

func BenchmarkAdaptiveOnPacketLoss(b *testing.B) {
	ac := NewAdaptive(nil, nil)
	ac.OnPacketSent(100000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ac.OnPacketLoss(1200)
	}
}
