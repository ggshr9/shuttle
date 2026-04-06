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
	if ac.switchCooldown != 3*time.Second {
		t.Fatalf("switchCooldown = %v, want 3s", ac.switchCooldown)
	}
	if ac.recoveryCooldown != 15*time.Second {
		t.Fatalf("recoveryCooldown = %v, want 15s", ac.recoveryCooldown)
	}
	if ac.detectionWindow != 3*time.Second {
		t.Fatalf("detectionWindow = %v, want 3s", ac.detectionWindow)
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
	n := ac.rttCount
	ac.mu.Unlock()

	if n != 1 {
		t.Fatalf("rttCount = %d, want 1", n)
	}
}

func TestAdaptiveRTTHistoryCap(t *testing.T) {
	ac := NewAdaptive(nil, nil)
	for i := 1; i <= 150; i++ {
		ac.OnAck(100, time.Duration(i)*time.Millisecond)
	}

	ac.mu.Lock()
	n := ac.rttCount
	ac.mu.Unlock()

	if n != 150 {
		t.Fatalf("rttCount = %d, want 150", n)
	}
	// Ring buffer is fixed-size array, no unbounded growth possible
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
		LossThreshold:    0.05,
		SwitchCooldown:   1 * time.Hour,
		RecoveryCooldown: 1 * time.Hour,
		BrutalRate:       10 * 1024 * 1024,
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
		t.Fatal("recoveryCooldown should prevent switch back to bbr")
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

func TestAdaptive_RTTHistoryMemoryBounded(t *testing.T) {
	ac := NewAdaptive(&AdaptiveConfig{
		LossThreshold:  0.05,
		SwitchCooldown: 1 * time.Nanosecond,
		BrutalRate:     100 * 1024 * 1024,
	}, nil)

	// Feed 10,000 RTT samples
	for i := 0; i < 10000; i++ {
		ac.OnAck(1200, time.Duration(50+i%20)*time.Millisecond)
	}

	ac.mu.Lock()
	count := ac.rttCount
	trend := ac.calculateRTTTrend()
	ac.mu.Unlock()

	// rttCount tracks total inserts but ring is bounded
	if count != 10000 {
		t.Errorf("rttCount=%d, want 10000", count)
	}
	// Trend should be computable without panic
	_ = trend
}

func TestRecordRTT_RejectsZero(t *testing.T) {
	ac := &AdaptiveCongestion{}
	ac.recordRTT(0)
	ac.recordRTT(-1 * time.Millisecond)
	if ac.rttCount != 0 {
		t.Errorf("zero/negative RTT should not be recorded, rttCount=%d", ac.rttCount)
	}
	ac.recordRTT(50 * time.Millisecond)
	if ac.rttCount != 1 {
		t.Errorf("valid RTT should be recorded, rttCount=%d, want 1", ac.rttCount)
	}
}

func TestCalculateRTTTrend_SparseRing_NoInflation(t *testing.T) {
	ac := &AdaptiveCongestion{}
	// Write exactly 25 samples (> 20 threshold, < 100 ring size)
	for i := 0; i < 25; i++ {
		ac.recordRTT(100 * time.Millisecond)
	}
	trend := ac.calculateRTTTrend()
	if trend < -0.05 || trend > 0.05 {
		t.Errorf("trend should be ~0 for uniform RTT, got %f", trend)
	}
}

// TestAsymmetricCooldown verifies that detection (BBR→Brutal) uses switchCooldown
// while recovery (Brutal→BBR) uses the longer recoveryCooldown.
func TestAsymmetricCooldown(t *testing.T) {
	ac := NewAdaptive(&AdaptiveConfig{
		LossThreshold:    0.05,
		SwitchCooldown:   1 * time.Nanosecond,  // instant detection
		RecoveryCooldown: 1 * time.Hour,         // very slow recovery
		DetectionWindow:  3 * time.Second,
		BrutalRate:       10 * 1024 * 1024,
	}, nil)

	// Seed stable RTT so rttTrend <= 0
	for i := 0; i < 30; i++ {
		ac.OnAck(1000, 50*time.Millisecond)
	}

	// Drive high loss → should switch to Brutal quickly (switchCooldown=1ns)
	ac.OnPacketSent(100000)
	for i := 0; i < 20; i++ {
		ac.OnPacketLoss(2000)
	}
	if ac.ActiveName() != "brutal" {
		t.Fatalf("expected brutal on interference, got %s", ac.ActiveName())
	}

	// Now simulate loss recovery — recoveryCooldown is 1h, so no switch back
	ac.mu.Lock()
	ac.lossRate = 0.01 // below threshold/2
	ac.evaluateSwitch()
	ac.mu.Unlock()

	if ac.ActiveName() != "brutal" {
		t.Fatalf("recoveryCooldown should block Brutal→BBR, got %s", ac.ActiveName())
	}

	// Force recoveryCooldown to expire and verify recovery is now allowed
	ac.mu.Lock()
	ac.recoveryCooldown = 1 * time.Nanosecond
	ac.lastSwitch = time.Now().Add(-time.Second) // past the nanosecond cooldown
	ac.lossRate = 0.01
	ac.evaluateSwitch()
	ac.mu.Unlock()

	if ac.ActiveName() != "bbr" {
		t.Fatalf("expected recovery to bbr after cooldown expired, got %s", ac.ActiveName())
	}
}

// TestAsymmetricCooldown_DetectionFasterThanRecovery verifies the full asymmetry:
// switchCooldown < recoveryCooldown means detection is faster than recovery.
func TestAsymmetricCooldown_DetectionFasterThanRecovery(t *testing.T) {
	switchCooldown := 1 * time.Nanosecond
	recoveryCooldown := 100 * time.Millisecond

	ac := NewAdaptive(&AdaptiveConfig{
		LossThreshold:    0.05,
		SwitchCooldown:   switchCooldown,
		RecoveryCooldown: recoveryCooldown,
		DetectionWindow:  3 * time.Second,
		BrutalRate:       10 * 1024 * 1024,
	}, nil)

	if ac.switchCooldown != switchCooldown {
		t.Fatalf("switchCooldown = %v, want %v", ac.switchCooldown, switchCooldown)
	}
	if ac.recoveryCooldown != recoveryCooldown {
		t.Fatalf("recoveryCooldown = %v, want %v", ac.recoveryCooldown, recoveryCooldown)
	}

	// The detection cooldown must be strictly less than recovery cooldown
	if ac.switchCooldown >= ac.recoveryCooldown {
		t.Fatalf("switchCooldown (%v) should be < recoveryCooldown (%v)", ac.switchCooldown, ac.recoveryCooldown)
	}
}

// TestAmbiguousZoneTimeout verifies that when loss is high but RTT trend is in
// the ambiguous range (0 < trend <= 0.1), the controller waits 5s then defaults
// to Brutal.
func TestAmbiguousZoneTimeout(t *testing.T) {
	ac := NewAdaptive(&AdaptiveConfig{
		LossThreshold:    0.05,
		SwitchCooldown:   1 * time.Nanosecond,
		RecoveryCooldown: 1 * time.Nanosecond,
		DetectionWindow:  3 * time.Second,
		BrutalRate:       10 * 1024 * 1024,
	}, nil)

	// Manually set state: high loss, ambiguous RTT trend (0 < trend <= 0.1)
	ac.mu.Lock()
	ac.lossRate = 0.10      // above threshold
	ac.rttTrend = 0.05      // ambiguous zone
	ac.lastSwitch = time.Time{} // no prior switch
	ac.evaluateSwitch()
	// Should have set ambiguousStart but NOT switched yet
	ambStart := ac.ambiguousStart
	active := ac.active
	ac.mu.Unlock()

	if ambStart.IsZero() {
		t.Fatal("ambiguousStart should be set on first ambiguous evaluation")
	}
	if active != ac.bbr {
		t.Fatalf("should still be on bbr during ambiguous zone, got %s", ac.ActiveName())
	}

	// Simulate time passage past 5s by backdating ambiguousStart
	ac.mu.Lock()
	ac.ambiguousStart = time.Now().Add(-6 * time.Second)
	ac.evaluateSwitch()
	ac.mu.Unlock()

	if ac.ActiveName() != "brutal" {
		t.Fatalf("expected brutal after ambiguous zone timeout, got %s", ac.ActiveName())
	}
}

// TestAmbiguousZoneClearedOnRecovery verifies ambiguousStart is cleared when
// loss drops below threshold.
func TestAmbiguousZoneClearedOnRecovery(t *testing.T) {
	ac := NewAdaptive(&AdaptiveConfig{
		LossThreshold:    0.05,
		SwitchCooldown:   1 * time.Nanosecond,
		RecoveryCooldown: 1 * time.Nanosecond,
		DetectionWindow:  3 * time.Second,
		BrutalRate:       10 * 1024 * 1024,
	}, nil)

	// Enter ambiguous zone
	ac.mu.Lock()
	ac.lossRate = 0.10
	ac.rttTrend = 0.05
	ac.evaluateSwitch()
	ambStart := ac.ambiguousStart
	ac.mu.Unlock()

	if ambStart.IsZero() {
		t.Fatal("ambiguousStart should be set in ambiguous zone")
	}

	// Recovery: drop loss below threshold/2
	ac.mu.Lock()
	ac.lossRate = 0.01
	ac.lastSwitch = time.Time{} // allow recovery
	ac.evaluateSwitch()
	cleared := ac.ambiguousStart.IsZero()
	ac.mu.Unlock()

	if !cleared {
		t.Fatal("ambiguousStart should be cleared on loss recovery")
	}
}

// TestDetectionWindowDefault verifies the detectionWindow default is 3s.
func TestDetectionWindowDefault(t *testing.T) {
	ac := NewAdaptive(nil, nil)
	if ac.detectionWindow != 3*time.Second {
		t.Fatalf("detectionWindow = %v, want 3s", ac.detectionWindow)
	}
}

// TestDetectionWindowCustom verifies the loss window uses detectionWindow field.
func TestDetectionWindowCustom(t *testing.T) {
	ac := NewAdaptive(&AdaptiveConfig{
		DetectionWindow: 500 * time.Millisecond,
		BrutalRate:      10 * 1024 * 1024,
	}, nil)
	if ac.detectionWindow != 500*time.Millisecond {
		t.Fatalf("detectionWindow = %v, want 500ms", ac.detectionWindow)
	}

	// Send some bytes, then backdate window to trigger reset
	ac.mu.Lock()
	ac.windowSentBytes = 9999
	ac.windowLostBytes = 9999
	ac.lastWindowReset = time.Now().Add(-time.Second) // past the 500ms window
	ac.resetWindowIfNeeded()
	sent := ac.windowSentBytes
	lost := ac.windowLostBytes
	ac.mu.Unlock()

	if sent != 0 || lost != 0 {
		t.Fatalf("window should have been reset after detectionWindow expired: sent=%d lost=%d", sent, lost)
	}
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
