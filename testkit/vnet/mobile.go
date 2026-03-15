// Package vnet mobile presets provide realistic LinkConfig profiles for common
// mobile network types, enabling tests that simulate mobile device behavior
// such as WiFi↔cellular handoffs, signal degradation, and network switching.
package vnet

import "time"

// --- Mobile Network Presets ---
// These return LinkConfig values based on empirical measurements of real-world
// mobile network characteristics.

// WiFi returns a typical WiFi link profile.
// ~5ms RTT, low jitter, negligible loss, 50 Mbps throughput.
func WiFi() LinkConfig {
	return LinkConfig{
		Latency:   2 * time.Millisecond, // one-way ≈ 2ms → 4ms RTT
		Jitter:    1 * time.Millisecond,
		Loss:      0.001,       // 0.1%
		Bandwidth: 50_000_000,  // 50 MB/s
	}
}

// WiFiCongested returns a congested WiFi link (coffee shop, conference).
// Higher latency, more jitter and loss due to contention.
func WiFiCongested() LinkConfig {
	return LinkConfig{
		Latency:   15 * time.Millisecond,
		Jitter:    10 * time.Millisecond,
		Loss:      0.03, // 3%
		Bandwidth: 2_000_000, // 2 MB/s
	}
}

// LTE returns a typical 4G/LTE link profile.
// ~30ms RTT, moderate jitter, low loss, 10 Mbps.
func LTE() LinkConfig {
	return LinkConfig{
		Latency:   15 * time.Millisecond, // one-way 15ms → 30ms RTT
		Jitter:    5 * time.Millisecond,
		Loss:      0.005, // 0.5%
		Bandwidth: 10_000_000, // 10 MB/s
	}
}

// FiveG returns a 5G sub-6GHz link profile.
// ~10ms RTT, low jitter, minimal loss, very high throughput.
func FiveG() LinkConfig {
	return LinkConfig{
		Latency:   5 * time.Millisecond,
		Jitter:    2 * time.Millisecond,
		Loss:      0.001,
		Bandwidth: 100_000_000, // 100 MB/s
	}
}

// ThreeG returns a 3G/HSPA link profile.
// ~80ms RTT, high jitter, moderate loss, limited bandwidth.
func ThreeG() LinkConfig {
	return LinkConfig{
		Latency:   40 * time.Millisecond, // one-way 40ms → 80ms RTT
		Jitter:    20 * time.Millisecond,
		Loss:      0.02, // 2%
		Bandwidth: 1_000_000, // 1 MB/s
	}
}

// Edge returns a 2G/EDGE link profile (worst-case mobile).
// ~300ms RTT, extreme jitter, high loss, very low bandwidth.
func Edge() LinkConfig {
	return LinkConfig{
		Latency:   150 * time.Millisecond,
		Jitter:    50 * time.Millisecond,
		Loss:      0.05, // 5%
		Bandwidth: 50_000, // 50 KB/s
	}
}

// CellularWeakSignal returns a cellular link with poor signal strength.
// High latency, significant jitter, notable packet loss.
func CellularWeakSignal() LinkConfig {
	return LinkConfig{
		Latency:      60 * time.Millisecond,
		Jitter:       40 * time.Millisecond,
		Loss:         0.08, // 8%
		Bandwidth:    500_000, // 500 KB/s
		ReorderPct:   0.05,
		ReorderDelay: 100 * time.Millisecond,
	}
}

// HandoffBlip returns a link config that simulates the brief disruption
// during a WiFi↔cellular handoff. High loss and latency spike for the
// transition period (typically 100-500ms in real devices).
func HandoffBlip() LinkConfig {
	return LinkConfig{
		Latency: 200 * time.Millisecond,
		Jitter:  100 * time.Millisecond,
		Loss:    0.30, // 30% loss during handoff
	}
}

// Subway returns a link simulating underground/tunnel connectivity:
// intermittent, high loss, variable latency.
func Subway() LinkConfig {
	return LinkConfig{
		Latency:      80 * time.Millisecond,
		Jitter:       60 * time.Millisecond,
		Loss:         0.15, // 15%
		Bandwidth:    200_000, // 200 KB/s
		ReorderPct:   0.1,
		ReorderDelay: 150 * time.Millisecond,
	}
}

// LinkDown returns a link config that drops all traffic (100% loss).
// Use with UpdateLink to simulate a network interface going down.
func LinkDown() LinkConfig {
	return LinkConfig{
		Loss: 1.0, // 100% loss = effectively down
	}
}
