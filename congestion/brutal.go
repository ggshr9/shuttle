package congestion

import (
	"sync"
	"time"
)

// BrutalController implements the Brutal congestion control algorithm.
// It maintains a fixed send rate regardless of packet loss, designed to
// overcome active interference (e.g., GFW random packet drops).
type BrutalController struct {
	mu sync.Mutex

	// Target send rate in bytes per second.
	targetRate uint64
	// Current congestion window.
	cwnd uint64
	// Acknowledgment tracking.
	totalSent  uint64
	totalAcked uint64
	totalLost  uint64
	// Current loss rate.
	lossRate float64
	// RTT estimate.
	rtt time.Duration
}

// NewBrutal creates a new Brutal congestion controller with the given target rate.
// rate is in bytes per second.
func NewBrutal(rate uint64) *BrutalController {
	if rate == 0 {
		rate = 100 * 1024 * 1024 // 100 Mbps default
	}
	return &BrutalController{
		targetRate: rate,
		cwnd:       rate, // Initial cwnd = 1 second of data
		rtt:        100 * time.Millisecond,
	}
}

// OnPacketSent records a packet being sent.
func (b *BrutalController) OnPacketSent(bytes uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.totalSent += bytes
}

// OnAck processes an acknowledgment.
func (b *BrutalController) OnAck(ackedBytes uint64, rtt time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.totalAcked += ackedBytes
	if rtt > 0 {
		// EWMA RTT update.
		if b.rtt == 0 {
			b.rtt = rtt
		} else {
			b.rtt = time.Duration(float64(b.rtt)*0.875 + float64(rtt)*0.125)
		}
	}
	// Decay loss rate toward 0 on successful acks
	b.lossRate *= 0.99
	b.updateCwnd()
}

// OnPacketLoss processes a packet loss event.
func (b *BrutalController) OnPacketLoss(lostBytes uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.totalLost += lostBytes
	// Use EWMA loss rate instead of cumulative ratio.
	// This allows the loss rate to decay when conditions improve.
	b.lossRate = b.lossRate*0.9 + 0.1
	b.updateCwnd()
}

func (b *BrutalController) updateCwnd() {
	// cwnd = targetRate * RTT / (1 - lossRate)
	// This compensates for loss by sending more.
	if b.rtt <= 0 {
		b.cwnd = b.targetRate
		return
	}
	rttSec := float64(b.rtt) / float64(time.Second)
	var lossFactor float64
	if b.lossRate < 0.99 {
		lossFactor = 1.0 / (1.0 - b.lossRate)
	} else {
		lossFactor = 100.0
	}
	b.cwnd = uint64(float64(b.targetRate) * rttSec * lossFactor)
	if b.cwnd < 4*1200 {
		b.cwnd = 4 * 1200
	}
}

// GetCwnd returns the current congestion window.
func (b *BrutalController) GetCwnd() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cwnd
}

// GetPacingRate returns the target send rate.
func (b *BrutalController) GetPacingRate() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.targetRate
}

// SetRate updates the target send rate.
func (b *BrutalController) SetRate(rate uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.targetRate = rate
	b.updateCwnd()
}

// Stats returns current Brutal statistics.
func (b *BrutalController) Stats() map[string]interface{} {
	b.mu.Lock()
	defer b.mu.Unlock()
	return map[string]interface{}{
		"targetRate": b.targetRate,
		"cwnd":       b.cwnd,
		"lossRate":   b.lossRate,
		"rtt":        b.rtt,
		"totalSent":  b.totalSent,
		"totalAcked": b.totalAcked,
		"totalLost":  b.totalLost,
	}
}
