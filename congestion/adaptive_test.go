package congestion

import (
	"testing"
	"time"
)

func TestAdaptiveInitialState(t *testing.T) {
	ac := NewAdaptive(&AdaptiveConfig{}, nil)
	if ac.ActiveName() != "bbr" {
		t.Fatalf("expected initial active = bbr, got %s", ac.ActiveName())
	}
	stats := ac.Stats()
	if stats["switchCount"].(int) != 0 {
		t.Fatalf("expected 0 switches, got %d", stats["switchCount"])
	}
}

func TestAdaptiveSwitchToBrutalOnInterference(t *testing.T) {
	ac := NewAdaptive(&AdaptiveConfig{
		LossThreshold:  0.05,
		SwitchCooldown: 1 * time.Nanosecond,
		BrutalRate:     10 * 1024 * 1024,
	}, nil)

	// Directly set state: high loss + stable RTT = interference → Brutal
	ac.mu.Lock()
	ac.lossRate = 0.10
	ac.rttTrend = 0 // stable
	ac.evaluateSwitch()
	ac.mu.Unlock()

	if ac.ActiveName() != "brutal" {
		t.Fatalf("expected switch to brutal on interference, got %s", ac.ActiveName())
	}
}

func TestAdaptiveSwitchToBBROnCongestion(t *testing.T) {
	ac := NewAdaptive(&AdaptiveConfig{
		LossThreshold:  0.05,
		SwitchCooldown: 1 * time.Nanosecond,
		BrutalRate:     10 * 1024 * 1024,
	}, nil)

	// Step 1: Switch to brutal
	ac.mu.Lock()
	ac.lossRate = 0.10
	ac.rttTrend = 0
	ac.evaluateSwitch()
	ac.mu.Unlock()
	if ac.ActiveName() != "brutal" {
		t.Fatalf("setup: expected brutal, got %s", ac.ActiveName())
	}

	// Step 2: Rising RTT + high loss → real congestion → BBR
	ac.mu.Lock()
	ac.lossRate = 0.10
	ac.rttTrend = 0.5 // rising
	ac.evaluateSwitch()
	ac.mu.Unlock()

	if ac.ActiveName() != "bbr" {
		t.Fatalf("expected switch to bbr on congestion (rising RTT), got %s", ac.ActiveName())
	}
}

func TestAdaptiveLossRecoveryToBBR(t *testing.T) {
	ac := NewAdaptive(&AdaptiveConfig{
		LossThreshold:  0.05,
		SwitchCooldown: 1 * time.Nanosecond,
		BrutalRate:     10 * 1024 * 1024,
	}, nil)

	// Switch to brutal
	ac.mu.Lock()
	ac.lossRate = 0.10
	ac.rttTrend = 0
	ac.evaluateSwitch()
	ac.mu.Unlock()
	if ac.ActiveName() != "brutal" {
		t.Fatalf("setup: expected brutal, got %s", ac.ActiveName())
	}

	// Loss recovery (below threshold/2) → BBR
	ac.mu.Lock()
	ac.lossRate = 0.01 // below 0.025
	ac.evaluateSwitch()
	ac.mu.Unlock()

	if ac.ActiveName() != "bbr" {
		t.Fatalf("expected switch back to bbr on loss recovery, got %s", ac.ActiveName())
	}
}

func TestAdaptiveCooldownPreventsRapidSwitch(t *testing.T) {
	ac := NewAdaptive(&AdaptiveConfig{
		LossThreshold:  0.05,
		SwitchCooldown: 1 * time.Hour,
		BrutalRate:     10 * 1024 * 1024,
	}, nil)

	// Set conditions that would normally trigger switch, but with recent lastSwitch
	ac.mu.Lock()
	ac.lossRate = 0.10
	ac.rttTrend = 0
	ac.lastSwitch = time.Now()
	ac.evaluateSwitch()
	ac.mu.Unlock()

	// Should still be BBR because cooldown hasn't elapsed
	if ac.ActiveName() != "bbr" {
		t.Fatalf("expected cooldown to prevent switch, still bbr, got %s", ac.ActiveName())
	}
}

func TestAdaptiveRTTTrendCalculation(t *testing.T) {
	ac := NewAdaptive(&AdaptiveConfig{}, nil)

	// Not enough samples → trend = 0
	for i := 0; i < 10; i++ {
		ac.mu.Lock()
		ac.recordRTT(50 * time.Millisecond)
		ac.mu.Unlock()
	}
	ac.mu.Lock()
	trend := ac.rttTrend
	ac.mu.Unlock()
	if trend != 0 {
		t.Fatalf("expected trend=0 with <20 samples, got %f", trend)
	}

	// Add more to reach 25 total — all same RTT → trend = 0
	for i := 0; i < 15; i++ {
		ac.mu.Lock()
		ac.recordRTT(50 * time.Millisecond)
		ac.mu.Unlock()
	}
	ac.mu.Lock()
	trend = ac.rttTrend
	ac.mu.Unlock()
	if trend != 0 {
		t.Fatalf("expected trend=0 with stable RTT, got %f", trend)
	}

	// Rising RTT
	for i := 0; i < 10; i++ {
		ac.mu.Lock()
		ac.recordRTT(100 * time.Millisecond)
		ac.mu.Unlock()
	}
	ac.mu.Lock()
	trend = ac.rttTrend
	ac.mu.Unlock()
	if trend <= 0 {
		t.Fatalf("expected positive trend with rising RTT, got %f", trend)
	}
}

func TestAdaptiveSwitchCount(t *testing.T) {
	ac := NewAdaptive(&AdaptiveConfig{
		LossThreshold:  0.05,
		SwitchCooldown: 1 * time.Nanosecond,
		BrutalRate:     10 * 1024 * 1024,
	}, nil)

	// Switch to brutal
	ac.mu.Lock()
	ac.lossRate = 0.10
	ac.rttTrend = 0
	ac.evaluateSwitch()
	ac.mu.Unlock()

	// Switch back to BBR
	ac.mu.Lock()
	ac.lossRate = 0.01
	ac.evaluateSwitch()
	ac.mu.Unlock()

	stats := ac.Stats()
	if stats["switchCount"].(int) != 2 {
		t.Fatalf("expected 2 switches, got %d", stats["switchCount"])
	}
}

func TestAdaptiveGetCwnd(t *testing.T) {
	ac := NewAdaptive(&AdaptiveConfig{BrutalRate: 50 * 1024 * 1024}, nil)

	cwnd := ac.GetCwnd()
	if cwnd == 0 {
		t.Fatal("expected non-zero cwnd")
	}
}

func TestAdaptiveLossWindowReset(t *testing.T) {
	ac := NewAdaptive(&AdaptiveConfig{SwitchCooldown: 0}, nil)

	// Send some bytes
	ac.OnPacketSent(5000)

	// Force window to be old
	ac.mu.Lock()
	ac.lastWindowReset = time.Now().Add(-15 * time.Second)
	ac.windowSentBytes = 9999
	ac.windowLostBytes = 8888
	ac.mu.Unlock()

	// Next send should reset the window
	ac.OnPacketSent(100)

	ac.mu.Lock()
	sent := ac.windowSentBytes
	lost := ac.windowLostBytes
	ac.mu.Unlock()

	if sent != 100 {
		t.Fatalf("expected window reset, sent=100, got %d", sent)
	}
	if lost != 0 {
		t.Fatalf("expected window reset, lost=0, got %d", lost)
	}
}

func TestAdaptiveOnPacketSentDelegates(t *testing.T) {
	ac := NewAdaptive(&AdaptiveConfig{}, nil)
	// Should not panic
	ac.OnPacketSent(1000)
	ac.OnAck(500, 10*time.Millisecond)
	ac.OnPacketLoss(100)
}

func TestAdaptiveEvaluateSwitchNoopWhenAlreadyActive(t *testing.T) {
	ac := NewAdaptive(&AdaptiveConfig{
		LossThreshold:  0.05,
		SwitchCooldown: 1 * time.Nanosecond,
	}, nil)

	// Already BBR, low loss → should try to switch to BBR (noop)
	ac.mu.Lock()
	ac.lossRate = 0.01
	initialCount := ac.switchCount
	ac.evaluateSwitch()
	finalCount := ac.switchCount
	ac.mu.Unlock()

	if finalCount != initialCount {
		t.Fatalf("expected no switch (already bbr), count changed from %d to %d", initialCount, finalCount)
	}
}

func TestAdaptiveStableRTTTrendIsZero(t *testing.T) {
	ac := NewAdaptive(&AdaptiveConfig{}, nil)

	ac.mu.Lock()
	ac.rttHistory = make([]time.Duration, 0, 100)
	for i := 0; i < 20; i++ {
		ac.rttHistory = append(ac.rttHistory, 50*time.Millisecond)
	}
	trend := ac.calculateRTTTrend()
	ac.mu.Unlock()

	if trend != 0 {
		t.Fatalf("expected 0 trend for stable RTT, got %f", trend)
	}
}
