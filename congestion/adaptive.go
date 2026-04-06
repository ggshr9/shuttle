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

const rttRingSize = 100

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
	lossRate        float64
	windowSentBytes uint64
	windowLostBytes uint64
	lastWindowReset time.Time
	rttRing         [rttRingSize]time.Duration
	rttCount        int     // total samples inserted (index = rttCount % rttRingSize)
	rttTrend        float64 // positive = rising, negative = falling
	switchCount     int
	lastSwitch      time.Time
	ambiguousStart  time.Time // set when in ambiguous loss/RTT zone

	// Thresholds.
	lossThreshold    float64       // Switch to Brutal above this loss rate
	switchCooldown   time.Duration // Cooldown for BBR→Brutal (fast interference detection)
	recoveryCooldown time.Duration // Cooldown for Brutal→BBR (slow recovery)
	detectionWindow  time.Duration // Loss measurement window

	logger *slog.Logger
}

// AdaptiveConfig configures the adaptive congestion controller.
type AdaptiveConfig struct {
	BrutalRate       uint64        // Target rate for Brutal mode
	LossThreshold    float64       // Loss rate threshold (default 0.05 = 5%)
	SwitchCooldown   time.Duration // Cooldown for BBR→Brutal detection (default 3s)
	RecoveryCooldown time.Duration // Cooldown for Brutal→BBR recovery (default 15s)
	DetectionWindow  time.Duration // Loss measurement window (default 3s)
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
		cfg.SwitchCooldown = 3 * time.Second
	}
	if cfg.RecoveryCooldown == 0 {
		cfg.RecoveryCooldown = 15 * time.Second
	}
	if cfg.DetectionWindow == 0 {
		cfg.DetectionWindow = 3 * time.Second
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
		bbr:              bbr,
		brutal:           brutal,
		active:           bbr, // Start with BBR
		lossThreshold:    cfg.LossThreshold,
		switchCooldown:   cfg.SwitchCooldown,
		recoveryCooldown: cfg.RecoveryCooldown,
		detectionWindow:  cfg.DetectionWindow,
		logger:           logger,
	}
}

// OnPacketSent delegates to the active controller and tracks bytes in the current window.
func (ac *AdaptiveCongestion) OnPacketSent(bytes uint64) {
	ac.mu.Lock()
	ac.resetWindowIfNeeded()
	ac.windowSentBytes += bytes
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
	if rtt <= 0 {
		return
	}
	ac.rttRing[ac.rttCount%rttRingSize] = rtt
	ac.rttCount++
	ac.rttTrend = ac.calculateRTTTrend()
}

func (ac *AdaptiveCongestion) calculateRTTTrend() float64 {
	count := ac.rttCount
	if count < 20 {
		return 0
	}

	// Use at most the number of valid samples, capped at rttRingSize
	validCount := count
	if validCount > rttRingSize {
		validCount = rttRingSize
	}
	halfWindow := validCount / 2
	if halfWindow < 5 {
		return 0
	}
	if halfWindow > 10 {
		halfWindow = 10
	}

	// Recent: last halfWindow samples
	var recentSum time.Duration
	for i := 0; i < halfWindow; i++ {
		idx := (count - 1 - i) % rttRingSize
		recentSum += ac.rttRing[idx]
	}
	recentAvg := recentSum / time.Duration(halfWindow)

	// Older: halfWindow samples before the recent ones
	var olderSum time.Duration
	for i := 0; i < halfWindow; i++ {
		idx := (count - 1 - halfWindow - i) % rttRingSize
		olderSum += ac.rttRing[idx]
	}
	olderAvg := olderSum / time.Duration(halfWindow)

	if olderAvg == 0 {
		return 0
	}
	return float64(recentAvg-olderAvg) / float64(olderAvg)
}

func (ac *AdaptiveCongestion) resetWindowIfNeeded() {
	window := ac.detectionWindow
	if window == 0 {
		window = 3 * time.Second
	}
	if time.Since(ac.lastWindowReset) >= window {
		ac.windowSentBytes = 0
		ac.windowLostBytes = 0
		ac.lastWindowReset = time.Now()
	}
}

func (ac *AdaptiveCongestion) updateLossRate(lostBytes uint64) {
	ac.resetWindowIfNeeded()
	ac.windowLostBytes += lostBytes
	if ac.windowSentBytes == 0 {
		return
	}
	// Compute actual loss ratio within the current window and blend with EMA.
	actualRatio := float64(ac.windowLostBytes) / float64(ac.windowSentBytes)
	ac.lossRate = ac.lossRate*0.8 + actualRatio*0.2
}

func (ac *AdaptiveCongestion) evaluateSwitch() {
	now := time.Now()

	switch {
	case ac.lossRate > ac.lossThreshold && ac.rttTrend <= 0:
		if now.Sub(ac.lastSwitch) < ac.switchCooldown {
			return
		}
		ac.switchTo(ac.brutal, "interference detected")
		ac.ambiguousStart = time.Time{}

	case ac.lossRate > ac.lossThreshold && ac.rttTrend > 0.1:
		if now.Sub(ac.lastSwitch) < ac.switchCooldown {
			return
		}
		ac.switchTo(ac.bbr, "real congestion")
		ac.ambiguousStart = time.Time{}

	case ac.lossRate > ac.lossThreshold && ac.rttTrend > 0 && ac.rttTrend <= 0.1:
		// Ambiguous zone: if persists > 5s, default to Brutal
		if ac.ambiguousStart.IsZero() {
			ac.ambiguousStart = now
		} else if now.Sub(ac.ambiguousStart) > 5*time.Second {
			if now.Sub(ac.lastSwitch) >= ac.switchCooldown {
				ac.switchTo(ac.brutal, "ambiguous zone timeout")
				ac.ambiguousStart = time.Time{}
			}
		}

	case ac.lossRate <= ac.lossThreshold/2:
		if now.Sub(ac.lastSwitch) < ac.recoveryCooldown {
			return
		}
		ac.switchTo(ac.bbr, "loss recovered")
		ac.ambiguousStart = time.Time{}
	}
}

func (ac *AdaptiveCongestion) switchTo(cc CongestionController, reason string) {
	if ac.active == cc {
		return
	}
	if cc == ac.bbr {
		ac.bbr.Reset()
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
	activeName := "bbr"
	if ac.active == ac.brutal {
		activeName = "brutal"
	}
	return map[string]interface{}{
		"active":      activeName,
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
