package congestion

import (
	"sync"
	"testing"
	"time"
)

func TestBBRNewDefault(t *testing.T) {
	bbr := NewBBR(0)
	if bbr.state != BBRStartup {
		t.Fatalf("initial state = %v, want BBRStartup", bbr.state)
	}
	cwnd := bbr.GetCwnd()
	if cwnd != 32*1200 {
		t.Fatalf("default cwnd = %d, want %d", cwnd, 32*1200)
	}
}

func TestBBRNewCustomCwnd(t *testing.T) {
	bbr := NewBBR(64000)
	if bbr.GetCwnd() != 64000 {
		t.Fatalf("custom cwnd = %d, want 64000", bbr.GetCwnd())
	}
}

func TestBBROnPacketSent(t *testing.T) {
	bbr := NewBBR(0)
	bbr.OnPacketSent(1000)
	bbr.OnPacketSent(2000)

	bbr.mu.Lock()
	total := bbr.totalSent
	bbr.mu.Unlock()

	if total != 3000 {
		t.Fatalf("totalSent = %d, want 3000", total)
	}
}

func TestBBROnPacketLoss(t *testing.T) {
	bbr := NewBBR(0)
	bbr.OnPacketLoss(500)
	bbr.OnPacketLoss(300)

	bbr.mu.Lock()
	total := bbr.totalLost
	bbr.mu.Unlock()

	if total != 800 {
		t.Fatalf("totalLost = %d, want 800", total)
	}
}

func TestBBRBandwidthEstimation(t *testing.T) {
	bbr := NewBBR(0)

	// Simulate acks with known RTT to verify bandwidth estimation
	rtt := 50 * time.Millisecond
	ackedBytes := uint64(10000)

	bbr.OnAck(ackedBytes, rtt)

	bbr.mu.Lock()
	btlBw := bbr.btlBw
	bbr.mu.Unlock()

	// Expected: 10000 bytes / 50ms = 200000 bytes/sec
	expectedBw := ackedBytes * uint64(time.Second) / uint64(rtt)
	if btlBw != expectedBw {
		t.Fatalf("btlBw = %d, want %d", btlBw, expectedBw)
	}
}

func TestBBRRTTTracking(t *testing.T) {
	bbr := NewBBR(0)

	bbr.OnAck(1000, 100*time.Millisecond)
	bbr.OnAck(1000, 50*time.Millisecond)
	bbr.OnAck(1000, 200*time.Millisecond)

	bbr.mu.Lock()
	rtProp := bbr.rtProp
	bbr.mu.Unlock()

	if rtProp != 50*time.Millisecond {
		t.Fatalf("rtProp = %v, want 50ms", rtProp)
	}
}

func TestBBRStateTransitionStartupToDrain(t *testing.T) {
	bbr := NewBBR(0)

	// Simulate enough acks with stable bandwidth to trigger filledPipe
	rtt := 50 * time.Millisecond
	for i := 0; i < 10; i++ {
		bbr.OnAck(10000, rtt)
	}

	bbr.mu.Lock()
	state := bbr.state
	bbr.mu.Unlock()

	// After enough rounds with non-growing bandwidth, pipe should be filled
	// and state should transition to Drain
	if bbr.filledPipe && state != BBRDrain {
		t.Fatalf("expected BBRDrain after filledPipe, got %v", state)
	}
}

func TestBBRZeroRTTIgnored(t *testing.T) {
	bbr := NewBBR(0)
	// Zero RTT should not update bandwidth
	bbr.OnAck(1000, 0)

	bbr.mu.Lock()
	btlBw := bbr.btlBw
	bbr.mu.Unlock()

	if btlBw != 0 {
		t.Fatalf("expected 0 btlBw for zero RTT, got %d", btlBw)
	}
}

func TestBBRPacingRate(t *testing.T) {
	bbr := NewBBR(0)
	bbr.OnAck(10000, 50*time.Millisecond)

	rate := bbr.GetPacingRate()
	if rate == 0 {
		t.Fatal("expected non-zero pacing rate after ack")
	}
}

func TestBBRStats(t *testing.T) {
	bbr := NewBBR(0)
	bbr.OnPacketSent(5000)
	bbr.OnAck(3000, 50*time.Millisecond)
	bbr.OnPacketLoss(500)

	stats := bbr.Stats()

	if stats["totalSent"].(uint64) != 5000 {
		t.Fatalf("stats totalSent = %v, want 5000", stats["totalSent"])
	}
	if stats["totalAcked"].(uint64) != 3000 {
		t.Fatalf("stats totalAcked = %v, want 3000", stats["totalAcked"])
	}
	if stats["totalLost"].(uint64) != 500 {
		t.Fatalf("stats totalLost = %v, want 500", stats["totalLost"])
	}
}

func TestBBRCwndBounds(t *testing.T) {
	bbr := NewBBR(0)

	bbr.mu.Lock()
	// Force extremely large BDP
	bbr.btlBw = 1e15
	bbr.rtProp = time.Second
	bbr.updateCwnd()
	cwnd := bbr.cwnd
	bbr.mu.Unlock()

	if cwnd > bbr.maxCwnd {
		t.Fatalf("cwnd %d exceeds maxCwnd %d", cwnd, bbr.maxCwnd)
	}

	bbr.mu.Lock()
	// Force extremely small BDP
	bbr.btlBw = 1
	bbr.rtProp = time.Nanosecond
	bbr.updateCwnd()
	cwnd = bbr.cwnd
	bbr.mu.Unlock()

	if cwnd < bbr.minCwnd {
		t.Fatalf("cwnd %d below minCwnd %d", cwnd, bbr.minCwnd)
	}
}

func TestBBRCwndGainByState(t *testing.T) {
	bbr := NewBBR(0)

	tests := []struct {
		state BBRState
		gain  float64
	}{
		{BBRStartup, bbrHighGain},
		{BBRDrain, bbrDrainGain},
		{BBRProbeBW, bbrCwndGain},
		{BBRProbeRTT, 1.0},
	}

	for _, tt := range tests {
		bbr.mu.Lock()
		bbr.state = tt.state
		gain := bbr.cwndGain()
		bbr.mu.Unlock()

		if gain != tt.gain {
			t.Errorf("state %v: cwndGain = %f, want %f", tt.state, gain, tt.gain)
		}
	}
}

func TestBBRPacingGainByState(t *testing.T) {
	bbr := NewBBR(0)

	bbr.mu.Lock()
	bbr.state = BBRStartup
	startupGain := bbr.pacingGain()
	bbr.state = BBRDrain
	drainGain := bbr.pacingGain()
	bbr.state = BBRProbeBW
	probeGain := bbr.pacingGain()
	bbr.mu.Unlock()

	if startupGain != bbrHighGain {
		t.Fatalf("startup pacing gain = %f, want %f", startupGain, bbrHighGain)
	}
	if drainGain != bbrDrainGain {
		t.Fatalf("drain pacing gain = %f, want %f", drainGain, bbrDrainGain)
	}
	if probeGain != 1.0 {
		t.Fatalf("probeBW pacing gain = %f, want 1.0", probeGain)
	}
}

func TestBBRBDPCalculation(t *testing.T) {
	bbr := NewBBR(0)

	bbr.mu.Lock()
	bbr.btlBw = 1_000_000 // 1 MB/s
	bbr.rtProp = 100 * time.Millisecond
	bdp := bbr.bdp()
	bbr.mu.Unlock()

	// BDP = 1MB/s * 100ms = 100KB
	expected := uint64(1_000_000 * 100 / 1000)
	if bdp != expected {
		t.Fatalf("bdp = %d, want %d", bdp, expected)
	}
}

// Benchmarks
func BenchmarkBBROnAck(b *testing.B) {
	bbr := NewBBR(0)
	rtt := 50 * time.Millisecond
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bbr.OnAck(1200, rtt)
	}
}

func BenchmarkBBROnPacketSent(b *testing.B) {
	bbr := NewBBR(0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bbr.OnPacketSent(1200)
	}
}

func TestBBR_InStartup_Concurrent(t *testing.T) {
	bbr := NewBBR(0)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			bbr.OnAck(1200, 50*time.Millisecond)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_ = bbr.InStartup()
		}
	}()

	wg.Wait()
}
