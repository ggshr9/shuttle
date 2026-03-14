package congestion

import (
	"testing"
	"time"
)

func TestBrutalNewDefault(t *testing.T) {
	b := NewBrutal(0)
	if b.GetPacingRate() != 100*1024*1024 {
		t.Fatalf("default rate = %d, want %d", b.GetPacingRate(), 100*1024*1024)
	}
}

func TestBrutalNewCustomRate(t *testing.T) {
	rate := uint64(50 * 1024 * 1024)
	b := NewBrutal(rate)
	if b.GetPacingRate() != rate {
		t.Fatalf("custom rate = %d, want %d", b.GetPacingRate(), rate)
	}
}

func TestBrutalOnPacketSent(t *testing.T) {
	b := NewBrutal(10 * 1024 * 1024)
	b.OnPacketSent(1200)
	b.OnPacketSent(1200)

	b.mu.Lock()
	total := b.totalSent
	b.mu.Unlock()

	if total != 2400 {
		t.Fatalf("totalSent = %d, want 2400", total)
	}
}

func TestBrutalOnAckRTTUpdate(t *testing.T) {
	b := NewBrutal(10 * 1024 * 1024)

	// NewBrutal sets initial rtt to 100ms. First ack applies EWMA.
	b.mu.Lock()
	initialRTT := b.rtt
	b.mu.Unlock()
	if initialRTT != 100*time.Millisecond {
		t.Fatalf("initial RTT = %v, want 100ms", initialRTT)
	}

	// First ack: EWMA(100ms, 50ms) = 100*0.875 + 50*0.125 = 87.5 + 6.25 = 93.75ms
	b.OnAck(1200, 50*time.Millisecond)

	b.mu.Lock()
	rtt1 := b.rtt
	b.mu.Unlock()

	expected1 := time.Duration(float64(100*time.Millisecond)*0.875 + float64(50*time.Millisecond)*0.125)
	if rtt1 != expected1 {
		t.Fatalf("RTT after first ack = %v, want %v", rtt1, expected1)
	}

	// Second ack: EWMA(93.75ms, 50ms)
	b.OnAck(1200, 50*time.Millisecond)

	b.mu.Lock()
	rtt2 := b.rtt
	b.mu.Unlock()

	expected2 := time.Duration(float64(expected1)*0.875 + float64(50*time.Millisecond)*0.125)
	if rtt2 != expected2 {
		t.Fatalf("RTT after second ack = %v, want %v", rtt2, expected2)
	}

	// RTT should be converging toward 50ms
	if rtt2 >= rtt1 {
		t.Fatalf("RTT should decrease toward 50ms: rtt1=%v, rtt2=%v", rtt1, rtt2)
	}
}

func TestBrutalDoesNotReduceCwndOnLoss(t *testing.T) {
	rate := uint64(10 * 1024 * 1024) // 10 MB/s
	b := NewBrutal(rate)

	// Set up known state
	b.OnAck(10000, 50*time.Millisecond)
	cwndBefore := b.GetCwnd()

	// Loss should NOT reduce cwnd — it should increase it (loss compensation)
	b.OnPacketLoss(5000)
	cwndAfter := b.GetCwnd()

	if cwndAfter < cwndBefore {
		t.Fatalf("Brutal reduced cwnd on loss: before=%d, after=%d", cwndBefore, cwndAfter)
	}
}

func TestBrutalLossCompensation(t *testing.T) {
	rate := uint64(10 * 1024 * 1024)
	b := NewBrutal(rate)

	// No loss
	b.OnAck(10000, 100*time.Millisecond)
	cwndNoLoss := b.GetCwnd()

	// Reset and add loss
	b2 := NewBrutal(rate)
	b2.OnAck(10000, 100*time.Millisecond)
	b2.OnPacketLoss(5000)          // 33% loss (5000 lost / 15000 total)
	cwndWithLoss := b2.GetCwnd()

	// With loss, cwnd should be higher (compensating)
	if cwndWithLoss <= cwndNoLoss {
		t.Fatalf("expected loss compensation: noLoss=%d, withLoss=%d", cwndNoLoss, cwndWithLoss)
	}
}

func TestBrutalSetRate(t *testing.T) {
	b := NewBrutal(10 * 1024 * 1024)

	newRate := uint64(50 * 1024 * 1024)
	b.SetRate(newRate)

	if b.GetPacingRate() != newRate {
		t.Fatalf("after SetRate: rate = %d, want %d", b.GetPacingRate(), newRate)
	}
}

func TestBrutalCwndFloor(t *testing.T) {
	b := NewBrutal(1) // Very low rate

	b.mu.Lock()
	b.rtt = time.Nanosecond
	b.updateCwnd()
	cwnd := b.cwnd
	b.mu.Unlock()

	minCwnd := uint64(4 * 1200)
	if cwnd < minCwnd {
		t.Fatalf("cwnd %d below minimum %d", cwnd, minCwnd)
	}
}

func TestBrutalHighLossRate(t *testing.T) {
	b := NewBrutal(10 * 1024 * 1024)

	// Simulate extreme loss (99%+)
	b.OnAck(100, 50*time.Millisecond)
	for i := 0; i < 100; i++ {
		b.OnPacketLoss(1000)
	}

	b.mu.Lock()
	lr := b.lossRate
	b.mu.Unlock()

	// Loss factor should be capped at 100x
	if lr < 0.99 {
		t.Fatalf("expected high loss rate, got %f", lr)
	}

	// cwnd should still be reasonable (not overflow)
	cwnd := b.GetCwnd()
	if cwnd == 0 {
		t.Fatal("cwnd should not be zero even with extreme loss")
	}
}

func TestBrutalZeroRTT(t *testing.T) {
	b := NewBrutal(10 * 1024 * 1024)

	b.mu.Lock()
	b.rtt = 0
	b.updateCwnd()
	cwnd := b.cwnd
	b.mu.Unlock()

	// With zero RTT, cwnd = targetRate
	if cwnd != 10*1024*1024 {
		t.Fatalf("zero RTT cwnd = %d, want %d", cwnd, 10*1024*1024)
	}
}

func TestBrutalStats(t *testing.T) {
	b := NewBrutal(10 * 1024 * 1024)
	b.OnPacketSent(5000)
	b.OnAck(3000, 50*time.Millisecond)
	b.OnPacketLoss(500)

	stats := b.Stats()

	if stats["totalSent"].(uint64) != 5000 {
		t.Fatalf("totalSent = %v, want 5000", stats["totalSent"])
	}
	if stats["totalAcked"].(uint64) != 3000 {
		t.Fatalf("totalAcked = %v, want 3000", stats["totalAcked"])
	}
	if stats["totalLost"].(uint64) != 500 {
		t.Fatalf("totalLost = %v, want 500", stats["totalLost"])
	}
	if stats["targetRate"].(uint64) != 10*1024*1024 {
		t.Fatalf("targetRate = %v, want %d", stats["targetRate"], 10*1024*1024)
	}
}

// Benchmarks
func BenchmarkBrutalOnAck(b *testing.B) {
	bc := NewBrutal(50 * 1024 * 1024)
	rtt := 50 * time.Millisecond
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bc.OnAck(1200, rtt)
	}
}

func BenchmarkBrutalOnPacketLoss(b *testing.B) {
	bc := NewBrutal(50 * 1024 * 1024)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bc.OnPacketLoss(1200)
	}
}
