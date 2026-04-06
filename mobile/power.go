package mobile

import (
	"sync"
)

// PowerState represents the device's current power status.
// Native code (Android/iOS) should call SetPowerState to update this
// as the battery level and charging state change.
type PowerState int

const (
	// PowerNormal is the default power state (battery > 20% or charging).
	PowerNormal PowerState = iota
	// PowerLow indicates the device is in low-power mode (battery 5-20%).
	PowerLow
	// PowerCritical indicates critically low battery (< 5%).
	PowerCritical
)

var (
	powerMu    sync.RWMutex
	powerState PowerState
	lowPowerCb func(PowerState)
)

// SetPowerState updates the device's power state. Native code should call this
// when the battery level changes or the user toggles low-power mode.
//
// state values: 0=Normal, 1=Low, 2=Critical
//
// Effects:
//   - Normal: Full keepalive frequency, all transports enabled
//   - Low: Reduced keepalive (2x interval), prefer CDN transport
//   - Critical: Minimal keepalive (4x interval), CDN only, no padding
func SetPowerState(state int) {
	ps := PowerState(state)

	powerMu.Lock()
	old := powerState
	powerState = ps
	cb := lowPowerCb
	powerMu.Unlock()

	if old != ps {
		mobileLogger.Info("power state changed", "from", old, "to", ps)
		if cb != nil {
			cb(ps)
		}
	}
}

// GetPowerState returns the current power state as an int.
// 0=Normal, 1=Low, 2=Critical
func GetPowerState() int {
	powerMu.RLock()
	defer powerMu.RUnlock()
	return int(powerState)
}

// OnPowerStateChange registers an internal callback for power state changes.
func OnPowerStateChange(cb func(PowerState)) {
	powerMu.Lock()
	lowPowerCb = cb
	powerMu.Unlock()
}

// currentPowerState returns the current power state (internal use).
func currentPowerState() PowerState {
	powerMu.RLock()
	defer powerMu.RUnlock()
	return powerState
}

// SetBatteryLevel is a convenience function that maps battery percentage
// to a PowerState and calls SetPowerState. Native code can use either
// SetBatteryLevel or SetPowerState directly.
//
// Mapping: 0-5% → Critical, 5-20% → Low, 20-100% → Normal
func SetBatteryLevel(percent int) {
	switch {
	case percent <= 5:
		SetPowerState(int(PowerCritical))
	case percent <= 20:
		SetPowerState(int(PowerLow))
	default:
		SetPowerState(int(PowerNormal))
	}
}
