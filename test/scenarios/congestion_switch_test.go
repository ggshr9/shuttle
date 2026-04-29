package scenarios

import (
	"testing"
	"time"

	"github.com/ggshr9/shuttle/congestion"
)

// ---------------------------------------------------------------------------
// TestAdaptiveSwitchOnLoss
//
// Start with a clean link (no loss). Feed the adaptive controller steady
// acks with stable RTT, then inject packet loss with stable/falling RTT
// (interference pattern). Verify the controller switches from BBR to Brutal.
// ---------------------------------------------------------------------------

func TestAdaptiveSwitchOnLoss(t *testing.T) {
	t.Parallel()

	ac := congestion.NewAdaptive(&congestion.AdaptiveConfig{
		LossThreshold:  0.05, // 5% loss triggers switch
		SwitchCooldown: 1 * time.Nanosecond, // near-zero cooldown for test
		BrutalRate:     10 * 1024 * 1024,
	}, nil)

	// Phase 1: Clean link — send packets with stable RTT, no loss.
	rtt := 50 * time.Millisecond
	for i := 0; i < 30; i++ {
		ac.OnPacketSent(1200)
		ac.OnAck(1200, rtt)
	}

	if name := ac.ActiveName(); name != "bbr" {
		t.Fatalf("phase 1: expected bbr on clean link, got %s", name)
	}

	// Phase 2: Inject packet loss with stable RTT (interference pattern).
	// Send a burst and then report significant loss.
	for i := 0; i < 20; i++ {
		ac.OnPacketSent(5000)
	}
	for i := 0; i < 40; i++ {
		ac.OnPacketLoss(3000)
	}

	if name := ac.ActiveName(); name != "brutal" {
		t.Fatalf("phase 2: expected switch to brutal on high loss + stable RTT, got %s", name)
	}
}

// ---------------------------------------------------------------------------
// TestAdaptiveRecoverOnImprove
//
// Start with a lossy link that triggers Brutal mode. Then improve conditions
// (low loss). Verify the controller adapts back to BBR.
// ---------------------------------------------------------------------------

func TestAdaptiveRecoverOnImprove(t *testing.T) {
	t.Parallel()

	ac := congestion.NewAdaptive(&congestion.AdaptiveConfig{
		LossThreshold:    0.05,
		SwitchCooldown:   1 * time.Nanosecond,
		RecoveryCooldown: 1 * time.Nanosecond,
		BrutalRate:       10 * 1024 * 1024,
	}, nil)

	// Build stable RTT history so rttTrend <= 0.
	rtt := 50 * time.Millisecond
	for i := 0; i < 30; i++ {
		ac.OnPacketSent(1200)
		ac.OnAck(1200, rtt)
	}

	// Inject loss to trigger switch to Brutal.
	for i := 0; i < 20; i++ {
		ac.OnPacketSent(5000)
	}
	for i := 0; i < 40; i++ {
		ac.OnPacketLoss(3000)
	}

	if name := ac.ActiveName(); name != "brutal" {
		t.Fatalf("pre-recovery: expected brutal, got %s", name)
	}

	// Phase 2: Improve conditions — send lots of data with no loss.
	// The loss rate uses EMA (0.8 * old + 0.2 * new) per window reset.
	// We need many send+ack cycles with zero loss, and the window must
	// reset (10s) so the loss rate recalculates. We simulate this by
	// doing many sends so the window bytes are large relative to lost bytes,
	// driving the actual ratio toward zero. Each OnPacketLoss call blends
	// the ratio via EMA. We need enough clean OnPacketLoss(0) equivalent
	// cycles — but OnPacketLoss always adds bytes. Instead, we just do
	// massive sends to dilute, then trigger a loss evaluation.
	for i := 0; i < 500; i++ {
		ac.OnPacketSent(50000)
		ac.OnAck(50000, rtt)
	}
	// Trigger evaluateSwitch via OnPacketLoss with a tiny amount.
	// The window now has 500*50000 = 25M sent and ~0 lost, so actual
	// ratio is near 0. EMA: 0.8 * old + 0.2 * ~0 → drops rapidly.
	for i := 0; i < 50; i++ {
		ac.OnPacketSent(50000)
		ac.OnPacketLoss(1) // negligible: 1 byte lost out of millions sent
	}

	if name := ac.ActiveName(); name != "bbr" {
		t.Fatalf("post-recovery: expected switch back to bbr, got %s", name)
	}
}

// ---------------------------------------------------------------------------
// TestCongestionMetrics
//
// Verify that the CC controllers report reasonable metrics after processing
// simulated traffic: bandwidth estimate, RTT, cwnd, pacing rate.
// ---------------------------------------------------------------------------

func TestCongestionMetrics(t *testing.T) {
	t.Parallel()

	t.Run("BBR", func(t *testing.T) {
		t.Parallel()
		bbr := congestion.NewBBR(0)

		// Simulate traffic: send and ack packets with realistic RTT.
		rtt := 50 * time.Millisecond
		for i := 0; i < 100; i++ {
			bbr.OnPacketSent(10000)
			bbr.OnAck(10000, rtt)
		}

		cwnd := bbr.GetCwnd()
		if cwnd == 0 {
			t.Fatal("BBR cwnd should be non-zero after traffic")
		}

		pacingRate := bbr.GetPacingRate()
		if pacingRate == 0 {
			t.Fatal("BBR pacing rate should be non-zero after traffic")
		}

		stats := bbr.Stats()
		if stats["totalSent"].(uint64) == 0 {
			t.Fatal("BBR totalSent should be non-zero")
		}
		if stats["totalAcked"].(uint64) == 0 {
			t.Fatal("BBR totalAcked should be non-zero")
		}

		t.Logf("BBR metrics: cwnd=%d, pacingRate=%d, stats=%v", cwnd, pacingRate, stats)
	})

	t.Run("Brutal", func(t *testing.T) {
		t.Parallel()
		targetRate := uint64(50 * 1024 * 1024) // 50 MB/s
		brutal := congestion.NewBrutal(targetRate)

		rtt := 100 * time.Millisecond
		for i := 0; i < 50; i++ {
			brutal.OnPacketSent(5000)
			brutal.OnAck(5000, rtt)
		}

		cwnd := brutal.GetCwnd()
		if cwnd == 0 {
			t.Fatal("Brutal cwnd should be non-zero")
		}

		pacingRate := brutal.GetPacingRate()
		if pacingRate != targetRate {
			t.Fatalf("Brutal pacing rate = %d, want %d (target rate)", pacingRate, targetRate)
		}

		stats := brutal.Stats()
		if stats["targetRate"].(uint64) != targetRate {
			t.Fatalf("Brutal targetRate in stats = %v, want %d", stats["targetRate"], targetRate)
		}

		t.Logf("Brutal metrics: cwnd=%d, pacingRate=%d", cwnd, pacingRate)
	})

	t.Run("Adaptive", func(t *testing.T) {
		t.Parallel()
		ac := congestion.NewAdaptive(&congestion.AdaptiveConfig{
			BrutalRate:     10 * 1024 * 1024,
			SwitchCooldown: 1 * time.Nanosecond,
		}, nil)

		rtt := 30 * time.Millisecond
		for i := 0; i < 100; i++ {
			ac.OnPacketSent(8000)
			ac.OnAck(8000, rtt)
		}

		cwnd := ac.GetCwnd()
		if cwnd == 0 {
			t.Fatal("Adaptive cwnd should be non-zero")
		}
		pacingRate := ac.GetPacingRate()
		if pacingRate == 0 {
			t.Fatal("Adaptive pacing rate should be non-zero")
		}

		stats := ac.Stats()
		if stats["active"].(string) != "bbr" {
			t.Fatalf("expected bbr active with low loss, got %s", stats["active"])
		}
		if _, ok := stats["lossRate"]; !ok {
			t.Fatal("missing lossRate in adaptive stats")
		}
		if _, ok := stats["switchCount"]; !ok {
			t.Fatal("missing switchCount in adaptive stats")
		}

		t.Logf("Adaptive metrics: active=%s, cwnd=%d, pacingRate=%d, stats=%v",
			ac.ActiveName(), cwnd, pacingRate, stats)
	})
}

// ---------------------------------------------------------------------------
// TestAdaptiveSwitchOnCongestion
//
// High loss + rising RTT = real congestion. Verify the adaptive CC stays on
// or switches to BBR (not Brutal).
// ---------------------------------------------------------------------------

func TestAdaptiveSwitchOnCongestion(t *testing.T) {
	t.Parallel()

	ac := congestion.NewAdaptive(&congestion.AdaptiveConfig{
		LossThreshold:  0.05,
		SwitchCooldown: 1 * time.Nanosecond,
		BrutalRate:     10 * 1024 * 1024,
	}, nil)

	// Build up rising RTT history (congestion pattern).
	for i := 0; i < 30; i++ {
		rtt := time.Duration(50+i*5) * time.Millisecond
		ac.OnPacketSent(1200)
		ac.OnAck(1200, rtt)
	}

	// Inject loss while RTT is rising.
	for i := 0; i < 20; i++ {
		ac.OnPacketSent(5000)
	}
	for i := 0; i < 40; i++ {
		ac.OnPacketLoss(3000)
	}

	// With rising RTT, the adaptive CC should recognize real congestion
	// and keep BBR active (not switch to Brutal).
	if name := ac.ActiveName(); name != "bbr" {
		t.Fatalf("expected bbr on real congestion (rising RTT), got %s", name)
	}
}

// ---------------------------------------------------------------------------
// TestBBRBandwidthEstimate
//
// Feed the BBR controller a known data rate and verify the bandwidth
// estimate converges to a reasonable value.
// ---------------------------------------------------------------------------

func TestBBRBandwidthEstimate(t *testing.T) {
	t.Parallel()

	bbr := congestion.NewBBR(0)

	// Simulate a 10 MB/s link: 10000 bytes acked every 1ms = 10 MB/s.
	rtt := 1 * time.Millisecond
	bytesPerAck := uint64(10000)
	for i := 0; i < 200; i++ {
		bbr.OnPacketSent(bytesPerAck)
		bbr.OnAck(bytesPerAck, rtt)
	}

	stats := bbr.Stats()
	btlBw := stats["btlBw"].(uint64)

	// Expected delivery rate = 10000 bytes / 1ms = 10,000,000 bytes/sec = 10 MB/s.
	expectedBw := uint64(10_000_000)
	// Allow some tolerance (BBR uses a max filter so it should be at or above).
	if btlBw < expectedBw/2 {
		t.Fatalf("BBR btlBw = %d, expected at least %d (half of theoretical)",
			btlBw, expectedBw/2)
	}
	t.Logf("BBR bandwidth estimate: %d bytes/sec (expected ~%d)", btlBw, expectedBw)
}

// ---------------------------------------------------------------------------
// TestBrutalLossCompensation
//
// Verify that Brutal increases its cwnd to compensate for packet loss,
// maintaining the target send rate.
// ---------------------------------------------------------------------------

func TestBrutalLossCompensation(t *testing.T) {
	t.Parallel()

	targetRate := uint64(10 * 1024 * 1024) // 10 MB/s
	brutal := congestion.NewBrutal(targetRate)

	rtt := 50 * time.Millisecond

	// Baseline cwnd with no loss.
	for i := 0; i < 50; i++ {
		brutal.OnPacketSent(5000)
		brutal.OnAck(5000, rtt)
	}
	cwndNoLoss := brutal.GetCwnd()

	// Now introduce 20% loss.
	for i := 0; i < 100; i++ {
		brutal.OnPacketSent(5000)
		if i%5 == 0 {
			brutal.OnPacketLoss(5000)
		} else {
			brutal.OnAck(5000, rtt)
		}
	}
	cwndWithLoss := brutal.GetCwnd()

	// Brutal should increase cwnd to compensate for loss.
	if cwndWithLoss <= cwndNoLoss {
		t.Fatalf("Brutal cwnd with loss (%d) should be > cwnd without loss (%d)",
			cwndWithLoss, cwndNoLoss)
	}

	// Pacing rate should remain at target.
	if rate := brutal.GetPacingRate(); rate != targetRate {
		t.Fatalf("Brutal pacing rate = %d, want %d (unchanged target)", rate, targetRate)
	}

	t.Logf("Brutal cwnd: no-loss=%d, with-loss=%d (compensated)", cwndNoLoss, cwndWithLoss)
}

// ---------------------------------------------------------------------------
// TestAdaptiveMultipleSwitchCycles
//
// Verify the adaptive CC can switch back and forth multiple times as
// conditions change, with proper switch counting.
// ---------------------------------------------------------------------------

func TestAdaptiveMultipleSwitchCycles(t *testing.T) {
	t.Parallel()

	ac := congestion.NewAdaptive(&congestion.AdaptiveConfig{
		LossThreshold:    0.05,
		SwitchCooldown:   1 * time.Nanosecond,
		RecoveryCooldown: 1 * time.Nanosecond,
		BrutalRate:       10 * 1024 * 1024,
	}, nil)

	rtt := 50 * time.Millisecond

	// Cycle 1: Clean -> interference (BBR -> Brutal).
	for i := 0; i < 30; i++ {
		ac.OnPacketSent(1200)
		ac.OnAck(1200, rtt)
	}
	for i := 0; i < 20; i++ {
		ac.OnPacketSent(5000)
	}
	for i := 0; i < 40; i++ {
		ac.OnPacketLoss(3000)
	}
	if name := ac.ActiveName(); name != "brutal" {
		t.Fatalf("cycle 1: expected brutal, got %s", name)
	}

	// Cycle 2: Recovery (Brutal -> BBR).
	// Send massive clean traffic to dilute loss rate via EMA.
	for i := 0; i < 500; i++ {
		ac.OnPacketSent(50000)
		ac.OnAck(50000, rtt)
	}
	// Trigger evaluateSwitch with negligible loss to drive EMA down.
	for i := 0; i < 50; i++ {
		ac.OnPacketSent(50000)
		ac.OnPacketLoss(1)
	}
	if name := ac.ActiveName(); name != "bbr" {
		t.Fatalf("cycle 2: expected bbr after recovery, got %s", name)
	}

	// Verify switch count >= 2 (at least one switch to brutal, one back to bbr).
	stats := ac.Stats()
	switchCount, ok := stats["switchCount"].(int)
	if !ok {
		t.Fatalf("switchCount type assertion failed: %T", stats["switchCount"])
	}
	if switchCount < 2 {
		t.Fatalf("expected at least 2 switches, got %d", switchCount)
	}
	t.Logf("completed %d switch cycles", switchCount)
}
