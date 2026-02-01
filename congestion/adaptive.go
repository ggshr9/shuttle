package congestion

import (
	"log/slog"
	"sync"
	"time"
)

// CongestionController is the common interface for congestion control algorithms.
type CongestionController interface {
	OnPacketSent(bytes uint64)
	OnAck(ackedBytes uint64, rtt time.Duration)
	OnPacketLoss(lostBytes uint64)
	GetCwnd() uint64
	GetPacingRate() uint64
}

// AdaptiveCongestion switches between BBR and Brutal based on network conditions.
// Key insight: high loss + stable RTT = active interference (use Brutal)
//
//	high loss + rising RTT = real congestion (use BBR)
type AdaptiveCongestion struct {
	mu sync.Mutex

	bbr    *BBRController
	brutal *BrutalController
	active CongestionController

	// Detection state.
	lossRate    float64
	rttHistory  []time.Duration
	rttTrend    float64 // positive = rising, negative = falling
	switchCount int
	lastSwitch  time.Time

	// Thresholds.
	lossThreshold  float64       // Switch to Brutal above this loss rate
	switchCooldown time.Duration // Minimum time between switches

	logger *slog.Logger
}

// AdaptiveConfig configures the adaptive congestion controller.
type AdaptiveConfig struct {
	BrutalRate     uint64        // Target rate for Brutal mode
	LossThreshold  float64       // Loss rate threshold (default 0.05 = 5%)
	SwitchCooldown time.Duration // Cooldown between switches (default 10s)
}

// NewAdaptive creates a new adaptive congestion controller.
func NewAdaptive(cfg *AdaptiveConfig, logger *slog.Logger) *AdaptiveCongestion {
	if cfg == nil {
		cfg = &AdaptiveConfig{}
	}
	if cfg.LossThreshold == 0 {
		cfg.LossThreshold = 0.05
	}
	if cfg.SwitchCooldown == 0 {
		cfg.SwitchCooldown = 10 * time.Second
	}
	if cfg.BrutalRate == 0 {
		cfg.BrutalRate = 100 * 1024 * 1024
	}
	if logger == nil {
		logger = slog.Default()
	}

	bbr := NewBBR(0)
	brutal := NewBrutal(cfg.BrutalRate)

	return &AdaptiveCongestion{
		bbr:            bbr,
		brutal:         brutal,
		active:         bbr, // Start with BBR
		lossThreshold:  cfg.LossThreshold,
		switchCooldown: cfg.SwitchCooldown,
		rttHistory:     make([]time.Duration, 0, 100),
		logger:         logger,
	}
}

// OnPacketSent delegates to the active controller.
func (ac *AdaptiveCongestion) OnPacketSent(bytes uint64) {
	ac.mu.Lock()
	active := ac.active
	ac.mu.Unlock()
	active.OnPacketSent(bytes)
}

// OnAck updates RTT tracking and delegates to active controller.
func (ac *AdaptiveCongestion) OnAck(ackedBytes uint64, rtt time.Duration) {
	ac.mu.Lock()
	ac.recordRTT(rtt)
	active := ac.active
	ac.mu.Unlock()
	active.OnAck(ackedBytes, rtt)
}

// OnPacketLoss evaluates whether to switch controllers.
func (ac *AdaptiveCongestion) OnPacketLoss(lostBytes uint64) {
	ac.mu.Lock()
	ac.updateLossRate(lostBytes)
	ac.evaluateSwitch()
	active := ac.active
	ac.mu.Unlock()
	active.OnPacketLoss(lostBytes)
}

func (ac *AdaptiveCongestion) recordRTT(rtt time.Duration) {
	ac.rttHistory = append(ac.rttHistory, rtt)
	if len(ac.rttHistory) > 100 {
		ac.rttHistory = ac.rttHistory[1:]
	}
	ac.rttTrend = ac.calculateRTTTrend()
}

func (ac *AdaptiveCongestion) calculateRTTTrend() float64 {
	n := len(ac.rttHistory)
	if n < 20 {
		return 0
	}
	// Compare recent RTTs (last 10) vs older RTTs (10 before that)
	recent := ac.rttHistory[n-10:]
	older := ac.rttHistory[n-20 : n-10]

	var recentAvg, olderAvg float64
	for _, r := range recent {
		recentAvg += float64(r)
	}
	for _, r := range older {
		olderAvg += float64(r)
	}
	recentAvg /= 10
	olderAvg /= 10

	if olderAvg == 0 {
		return 0
	}
	return (recentAvg - olderAvg) / olderAvg
}

func (ac *AdaptiveCongestion) updateLossRate(lostBytes uint64) {
	// Simple exponential moving average of loss events.
	if lostBytes > 0 {
		ac.lossRate = ac.lossRate*0.8 + 0.2
	} else {
		ac.lossRate = ac.lossRate * 0.95
	}
}

func (ac *AdaptiveCongestion) evaluateSwitch() {
	// Cooldown check.
	if time.Since(ac.lastSwitch) < ac.switchCooldown {
		return
	}

	if ac.lossRate > ac.lossThreshold && ac.rttTrend <= 0 {
		// High loss + stable/falling RTT = active interference → Brutal
		ac.switchTo(ac.brutal, "GFW interference detected")
	} else if ac.lossRate > ac.lossThreshold && ac.rttTrend > 0.1 {
		// High loss + rising RTT = real congestion → BBR
		ac.switchTo(ac.bbr, "real congestion detected")
	} else if ac.lossRate <= ac.lossThreshold/2 {
		// Low loss → BBR (more efficient)
		ac.switchTo(ac.bbr, "loss recovered")
	}
}

func (ac *AdaptiveCongestion) switchTo(cc CongestionController, reason string) {
	if ac.active == cc {
		return
	}
	name := "bbr"
	if cc == ac.brutal {
		name = "brutal"
	}
	ac.logger.Info("switching congestion controller",
		"to", name,
		"reason", reason,
		"lossRate", ac.lossRate,
		"rttTrend", ac.rttTrend)
	ac.active = cc
	ac.lastSwitch = time.Now()
	ac.switchCount++
}

// GetCwnd returns the active controller's congestion window.
func (ac *AdaptiveCongestion) GetCwnd() uint64 {
	ac.mu.Lock()
	active := ac.active
	ac.mu.Unlock()
	return active.GetCwnd()
}

// GetPacingRate returns the active controller's pacing rate.
func (ac *AdaptiveCongestion) GetPacingRate() uint64 {
	ac.mu.Lock()
	active := ac.active
	ac.mu.Unlock()
	return active.GetPacingRate()
}

// ActiveName returns the name of the active controller.
func (ac *AdaptiveCongestion) ActiveName() string {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if ac.active == ac.brutal {
		return "brutal"
	}
	return "bbr"
}

// Stats returns combined statistics.
func (ac *AdaptiveCongestion) Stats() map[string]interface{} {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	return map[string]interface{}{
		"active":      ac.ActiveName(),
		"lossRate":    ac.lossRate,
		"rttTrend":    ac.rttTrend,
		"switchCount": ac.switchCount,
		"bbr":         ac.bbr.Stats(),
		"brutal":      ac.brutal.Stats(),
	}
}

var _ CongestionController = (*AdaptiveCongestion)(nil)
var _ CongestionController = (*BBRController)(nil)
var _ CongestionController = (*BrutalController)(nil)
