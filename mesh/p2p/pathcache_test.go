package p2p

import (
	"net"
	"testing"
	"time"
)

func TestPathCache(t *testing.T) {
	cache := NewPathCache(time.Hour)

	peerVIP := net.ParseIP("10.7.0.2")
	remoteAddr := &net.UDPAddr{IP: net.ParseIP("203.0.113.1"), Port: 12345}

	// Initially empty
	if cache.Get(peerVIP) != nil {
		t.Error("expected nil for unknown peer")
	}

	// Record success
	cache.RecordSuccess(peerVIP, remoteAddr, MethodSTUN, 50*time.Millisecond)

	entry := cache.Get(peerVIP)
	if entry == nil {
		t.Fatal("expected entry after recording success")
	}

	if entry.Method != MethodSTUN {
		t.Errorf("Method = %v, want MethodSTUN", entry.Method)
	}
	if entry.RTT != 50*time.Millisecond {
		t.Errorf("RTT = %v, want 50ms", entry.RTT)
	}
	if entry.SuccessCount != 1 {
		t.Errorf("SuccessCount = %d, want 1", entry.SuccessCount)
	}
}

func TestPathCacheGetBestMethod(t *testing.T) {
	cache := NewPathCache(time.Hour)

	peerVIP := net.ParseIP("10.7.0.2")
	remoteAddr := &net.UDPAddr{IP: net.ParseIP("203.0.113.1"), Port: 12345}

	// No data - unknown
	if cache.GetBestMethod(peerVIP) != MethodUnknown {
		t.Error("expected MethodUnknown for no data")
	}

	// Record success
	cache.RecordSuccess(peerVIP, remoteAddr, MethodUPnP, 30*time.Millisecond)

	if cache.GetBestMethod(peerVIP) != MethodUPnP {
		t.Error("expected MethodUPnP after success")
	}
}

func TestPathCacheFailure(t *testing.T) {
	cache := NewPathCache(time.Hour)

	peerVIP := net.ParseIP("10.7.0.2")

	cache.RecordFailure(peerVIP)

	entry := cache.Get(peerVIP)
	// Entry exists but no success time, so Get returns nil
	if entry != nil {
		t.Error("expected nil for peer with only failures")
	}
}

func TestPathCacheStats(t *testing.T) {
	cache := NewPathCache(time.Hour)

	peer1 := net.ParseIP("10.7.0.2")
	peer2 := net.ParseIP("10.7.0.3")
	addr := &net.UDPAddr{IP: net.ParseIP("203.0.113.1"), Port: 12345}

	cache.RecordSuccess(peer1, addr, MethodSTUN, 50*time.Millisecond)
	cache.RecordSuccess(peer2, addr, MethodUPnP, 30*time.Millisecond)

	stats := cache.Stats()
	if stats.TotalEntries != 2 {
		t.Errorf("TotalEntries = %d, want 2", stats.TotalEntries)
	}
	if stats.ByMethod[MethodSTUN] != 1 {
		t.Errorf("ByMethod[STUN] = %d, want 1", stats.ByMethod[MethodSTUN])
	}
	if stats.ByMethod[MethodUPnP] != 1 {
		t.Errorf("ByMethod[UPnP] = %d, want 1", stats.ByMethod[MethodUPnP])
	}
}

func TestPathCacheClear(t *testing.T) {
	cache := NewPathCache(time.Hour)

	peerVIP := net.ParseIP("10.7.0.2")
	addr := &net.UDPAddr{IP: net.ParseIP("203.0.113.1"), Port: 12345}

	cache.RecordSuccess(peerVIP, addr, MethodSTUN, 50*time.Millisecond)
	cache.Clear()

	if cache.Get(peerVIP) != nil {
		t.Error("expected nil after clear")
	}
}

func TestConnectionQuality(t *testing.T) {
	q := NewConnectionQuality()

	// Record some RTT samples
	for i := 0; i < 10; i++ {
		q.RecordRTT(time.Duration(50+i*5) * time.Millisecond)
	}

	q.RecordPacketSent()
	q.RecordPacketSent()
	q.RecordPacketReceived()
	q.RecordPacketLost()

	metrics := q.GetMetrics()

	if metrics.PacketsSent != 2 {
		t.Errorf("PacketsSent = %d, want 2", metrics.PacketsSent)
	}
	if metrics.PacketsReceived != 1 {
		t.Errorf("PacketsReceived = %d, want 1", metrics.PacketsReceived)
	}
	if metrics.PacketsLost != 1 {
		t.Errorf("PacketsLost = %d, want 1", metrics.PacketsLost)
	}

	// Check RTT is in expected range
	if metrics.AvgRTT < 50*time.Millisecond || metrics.AvgRTT > 100*time.Millisecond {
		t.Errorf("AvgRTT = %v, expected 50-100ms", metrics.AvgRTT)
	}

	if metrics.MinRTT != 50*time.Millisecond {
		t.Errorf("MinRTT = %v, want 50ms", metrics.MinRTT)
	}

	// Score should be reduced due to loss
	if metrics.Score >= 100 {
		t.Errorf("Score = %d, expected < 100 due to packet loss", metrics.Score)
	}
}

func TestQualityMetricsIsGood(t *testing.T) {
	// Good quality
	good := QualityMetrics{Score: 80, LossRate: 0.01}
	if !good.IsGood() {
		t.Error("expected IsGood() = true for good quality")
	}

	// Bad score
	badScore := QualityMetrics{Score: 30, LossRate: 0.01}
	if badScore.IsGood() {
		t.Error("expected IsGood() = false for low score")
	}

	// High loss
	highLoss := QualityMetrics{Score: 80, LossRate: 0.10}
	if highLoss.IsGood() {
		t.Error("expected IsGood() = false for high loss")
	}
}

func TestConnectionMethodString(t *testing.T) {
	tests := []struct {
		method ConnectionMethod
		want   string
	}{
		{MethodUnknown, "unknown"},
		{MethodUPnP, "upnp"},
		{MethodNATPMP, "nat-pmp"},
		{MethodSTUN, "stun"},
		{MethodDirect, "direct"},
		{MethodRelay, "relay"},
	}

	for _, tt := range tests {
		if got := tt.method.String(); got != tt.want {
			t.Errorf("%v.String() = %q, want %q", tt.method, got, tt.want)
		}
	}
}
