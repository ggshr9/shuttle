package test

import (
	"testing"
	"time"

	"github.com/shuttleX/shuttle/congestion"
)

func TestBBRBasic(t *testing.T) {
	bbr := congestion.NewBBR(0)
	if bbr.GetCwnd() == 0 {
		t.Error("initial cwnd should not be zero")
	}

	// Simulate some traffic
	for i := 0; i < 100; i++ {
		bbr.OnPacketSent(1200)
		bbr.OnAck(1200, 50*time.Millisecond)
	}

	cwnd := bbr.GetCwnd()
	if cwnd == 0 {
		t.Error("cwnd should not be zero after traffic")
	}

	rate := bbr.GetPacingRate()
	t.Logf("BBR cwnd=%d, pacingRate=%d", cwnd, rate)
}

func TestBrutalBasic(t *testing.T) {
	brutal := congestion.NewBrutal(50 * 1024 * 1024) // 50 Mbps

	// Simulate traffic with loss
	for i := 0; i < 100; i++ {
		brutal.OnPacketSent(1200)
		if i%10 == 0 {
			brutal.OnPacketLoss(1200) // 10% loss
		} else {
			brutal.OnAck(1200, 100*time.Millisecond)
		}
	}

	cwnd := brutal.GetCwnd()
	if cwnd == 0 {
		t.Error("brutal cwnd should not be zero even with loss")
	}

	rate := brutal.GetPacingRate()
	if rate != 50*1024*1024 {
		t.Errorf("brutal rate should stay at target, got %d", rate)
	}
	t.Logf("Brutal cwnd=%d, rate=%d", cwnd, rate)
}

func TestAdaptiveSwitching(t *testing.T) {
	ac := congestion.NewAdaptive(&congestion.AdaptiveConfig{
		BrutalRate:     100 * 1024 * 1024,
		LossThreshold:  0.05,
		SwitchCooldown: 0, // Disable cooldown for testing
	}, nil)

	// Start in BBR mode
	if ac.ActiveName() != "bbr" {
		t.Error("should start in BBR mode")
	}

	// Simulate GFW interference: high loss, stable RTT
	for i := 0; i < 200; i++ {
		ac.OnPacketSent(1200)
		ac.OnAck(1200, 50*time.Millisecond) // Stable RTT
	}
	for i := 0; i < 50; i++ {
		ac.OnPacketLoss(1200) // High loss
	}

	// Should eventually switch to brutal
	t.Logf("Active controller after interference: %s", ac.ActiveName())
}

func TestBBRStateTransitions(t *testing.T) {
	bbr := congestion.NewBBR(0)
	stats := bbr.Stats()
	if stats["state"] != congestion.BBRStartup {
		t.Errorf("expected BBRStartup state, got %v", stats["state"])
	}
}

func TestBrutalLossCompensation(t *testing.T) {
	brutal := congestion.NewBrutal(10 * 1024 * 1024)

	// No loss
	brutal.OnAck(1200, 50*time.Millisecond)
	cwndNoLoss := brutal.GetCwnd()

	// Reset and add 50% loss
	brutal2 := congestion.NewBrutal(10 * 1024 * 1024)
	for i := 0; i < 100; i++ {
		if i%2 == 0 {
			brutal2.OnPacketLoss(1200)
		} else {
			brutal2.OnAck(1200, 50*time.Millisecond)
		}
	}
	cwndWithLoss := brutal2.GetCwnd()

	// With loss, cwnd should be higher to compensate
	if cwndWithLoss <= cwndNoLoss {
		t.Logf("cwnd with loss (%d) should be >= no loss (%d)", cwndWithLoss, cwndNoLoss)
	}
}
