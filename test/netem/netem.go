// Package netem provides a Go helper for Docker-based network impairment
// using Linux tc netem. It is intended for use in sandbox integration tests.
package netem

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Impairment defines network condition parameters for tc netem.
type Impairment struct {
	Delay     time.Duration // Added latency
	Jitter    time.Duration // Latency variance
	Loss      float64       // Packet loss percentage (0-100)
	Bandwidth string        // Rate limit (e.g., "1mbit", "10mbit")
	Reorder   float64       // Packet reorder percentage
	Duplicate float64       // Packet duplication percentage
	Corrupt   float64       // Packet corruption percentage
}

// DefaultRouter is the container name used by ApplyToRouter / ResetRouter.
const DefaultRouter = "shuttle-router"

// DefaultIface is the default network interface used by ApplyToRouter / ResetRouter.
const DefaultIface = "eth0"

// HighLatency returns an impairment with 200ms delay and 20ms jitter.
func HighLatency() Impairment {
	return Impairment{
		Delay:  200 * time.Millisecond,
		Jitter: 20 * time.Millisecond,
	}
}

// PacketLoss returns an impairment with the specified loss percentage.
func PacketLoss(pct float64) Impairment {
	return Impairment{
		Loss: pct,
	}
}

// Satellite returns an impairment simulating a satellite link:
// 500ms delay, 50ms jitter, 5% loss.
func Satellite() Impairment {
	return Impairment{
		Delay:  500 * time.Millisecond,
		Jitter: 50 * time.Millisecond,
		Loss:   5,
	}
}

// GFWSimulation returns an impairment simulating active interference
// (Great Firewall-like): 50ms delay, 10% loss, 2% reorder.
func GFWSimulation() Impairment {
	return Impairment{
		Delay:   50 * time.Millisecond,
		Loss:    10,
		Reorder: 2,
	}
}

// SlowLink returns an impairment simulating a slow link:
// 100ms delay, 1mbit bandwidth cap.
func SlowLink() Impairment {
	return Impairment{
		Delay:     100 * time.Millisecond,
		Bandwidth: "1mbit",
	}
}

// Pristine returns a zero-value Impairment (no impairment), useful for reset.
func Pristine() Impairment {
	return Impairment{}
}

// --- Mobile Network Presets ---

// WiFi returns an impairment simulating a typical WiFi connection:
// 5ms delay, 2ms jitter, 50mbit bandwidth.
func WiFi() Impairment {
	return Impairment{
		Delay:     5 * time.Millisecond,
		Jitter:    2 * time.Millisecond,
		Bandwidth: "50mbit",
	}
}

// WiFiCongested returns an impairment simulating a congested WiFi link
// (e.g. coffee shop, conference): 30ms delay, 20ms jitter, 3% loss, 2mbit.
func WiFiCongested() Impairment {
	return Impairment{
		Delay:     30 * time.Millisecond,
		Jitter:    20 * time.Millisecond,
		Loss:      3,
		Bandwidth: "2mbit",
	}
}

// LTE returns an impairment simulating a 4G/LTE connection:
// 30ms delay, 10ms jitter, 0.5% loss, 10mbit.
func LTE() Impairment {
	return Impairment{
		Delay:     30 * time.Millisecond,
		Jitter:    10 * time.Millisecond,
		Loss:      0.5,
		Bandwidth: "10mbit",
	}
}

// FiveG returns an impairment simulating a 5G sub-6GHz connection:
// 10ms delay, 4ms jitter, 100mbit.
func FiveG() Impairment {
	return Impairment{
		Delay:     10 * time.Millisecond,
		Jitter:    4 * time.Millisecond,
		Bandwidth: "100mbit",
	}
}

// ThreeG returns an impairment simulating a 3G/HSPA connection:
// 80ms delay, 40ms jitter, 2% loss, 1mbit.
func ThreeG() Impairment {
	return Impairment{
		Delay:     80 * time.Millisecond,
		Jitter:    40 * time.Millisecond,
		Loss:      2,
		Bandwidth: "1mbit",
	}
}

// MobileEdge returns an impairment simulating a 2G/EDGE connection:
// 300ms delay, 100ms jitter, 5% loss, 50kbit.
func MobileEdge() Impairment {
	return Impairment{
		Delay:     300 * time.Millisecond,
		Jitter:    100 * time.Millisecond,
		Loss:      5,
		Bandwidth: "50kbit",
	}
}

// CellularWeakSignal returns an impairment simulating poor cellular signal:
// 120ms delay, 80ms jitter, 8% loss, 5% reorder, 500kbit.
func CellularWeakSignal() Impairment {
	return Impairment{
		Delay:     120 * time.Millisecond,
		Jitter:    80 * time.Millisecond,
		Loss:      8,
		Reorder:   5,
		Bandwidth: "500kbit",
	}
}

// HandoffBlip returns an impairment simulating the disruption during
// a WiFi↔cellular handoff: 400ms delay, 200ms jitter, 30% loss.
func HandoffBlip() Impairment {
	return Impairment{
		Delay:  400 * time.Millisecond,
		Jitter: 200 * time.Millisecond,
		Loss:   30,
	}
}

// Subway returns an impairment simulating underground/tunnel connectivity:
// 160ms delay, 120ms jitter, 15% loss, 10% reorder, 200kbit.
func Subway() Impairment {
	return Impairment{
		Delay:     160 * time.Millisecond,
		Jitter:    120 * time.Millisecond,
		Loss:      15,
		Reorder:   10,
		Bandwidth: "200kbit",
	}
}

// Apply runs tc netem on a container's interface via docker exec.
// It first attempts "tc qdisc add"; if a qdisc already exists it falls back
// to "tc qdisc change" so callers can apply repeatedly without resetting first.
func Apply(container, iface string, imp Impairment) error {
	args := buildNetemArgs(imp)
	if len(args) == 0 {
		// No impairment fields set — treat as a reset.
		return Reset(container, iface)
	}

	// Try "add" first; fall back to "change" if a root qdisc already exists.
	addCmd := append([]string{
		"exec", container, "tc", "qdisc", "add", "dev", iface, "root", "netem",
	}, args...)

	if err := dockerExec(addCmd...); err != nil {
		changeCmd := append([]string{
			"exec", container, "tc", "qdisc", "change", "dev", iface, "root", "netem",
		}, args...)
		if err2 := dockerExec(changeCmd...); err2 != nil {
			return fmt.Errorf("netem apply failed (add: %v, change: %v)", err, err2)
		}
	}

	// If a bandwidth cap is requested we need an additional tbf qdisc as a
	// child of netem because netem itself does not support rate limiting.
	if imp.Bandwidth != "" {
		if err := applyBandwidth(container, iface, imp.Bandwidth); err != nil {
			return fmt.Errorf("bandwidth apply failed: %w", err)
		}
	}

	return nil
}

// Reset removes all tc qdiscs from a container's interface.
func Reset(container, iface string) error {
	// "tc qdisc del ... root" removes whatever root qdisc exists.
	// If there is no qdisc, the command returns an error which we ignore.
	_ = dockerExec("exec", container, "tc", "qdisc", "del", "dev", iface, "root")
	return nil
}

// ApplyToRouter is a convenience that applies impairment to the router
// container's default interface (eth0).
func ApplyToRouter(imp Impairment) error {
	return Apply(DefaultRouter, DefaultIface, imp)
}

// ResetRouter removes impairment from the router container's default interface.
func ResetRouter() error {
	return Reset(DefaultRouter, DefaultIface)
}

// buildNetemArgs constructs the argument slice for the netem qdisc from the
// Impairment fields. Bandwidth is handled separately via a tbf child qdisc.
func buildNetemArgs(imp Impairment) []string {
	var args []string

	if imp.Delay > 0 {
		args = append(args, "delay", fmtDuration(imp.Delay))
		if imp.Jitter > 0 {
			args = append(args, fmtDuration(imp.Jitter))
		}
	}

	if imp.Loss > 0 {
		args = append(args, "loss", fmt.Sprintf("%.2f%%", imp.Loss))
	}

	if imp.Reorder > 0 {
		args = append(args, "reorder", fmt.Sprintf("%.2f%%", imp.Reorder))
	}

	if imp.Duplicate > 0 {
		args = append(args, "duplicate", fmt.Sprintf("%.2f%%", imp.Duplicate))
	}

	if imp.Corrupt > 0 {
		args = append(args, "corrupt", fmt.Sprintf("%.2f%%", imp.Corrupt))
	}

	return args
}

// applyBandwidth adds a tbf (token bucket filter) qdisc as a child of the
// netem qdisc to enforce a bandwidth cap.
func applyBandwidth(container, iface, rate string) error {
	// Parent 1:1 is the netem qdisc; tbf sits beneath it.
	// burst and latency are required parameters for tbf.
	addCmd := []string{
		"exec", container, "tc", "qdisc", "add", "dev", iface,
		"parent", "1:1", "handle", "10:",
		"tbf", "rate", rate, "burst", "32kbit", "latency", "400ms",
	}
	if err := dockerExec(addCmd...); err != nil {
		changeCmd := []string{
			"exec", container, "tc", "qdisc", "change", "dev", iface,
			"parent", "1:1", "handle", "10:",
			"tbf", "rate", rate, "burst", "32kbit", "latency", "400ms",
		}
		if err2 := dockerExec(changeCmd...); err2 != nil {
			return fmt.Errorf("tbf add: %v, change: %v", err, err2)
		}
	}
	return nil
}

// fmtDuration formats a time.Duration as a string suitable for tc netem
// (e.g. "200ms", "1s").
func fmtDuration(d time.Duration) string {
	ms := d.Milliseconds()
	if ms > 0 {
		return fmt.Sprintf("%dms", ms)
	}
	// Sub-millisecond: use microseconds.
	return fmt.Sprintf("%dus", d.Microseconds())
}

// dockerExec runs "docker <args...>" and returns any error. Standard error
// output is captured and included in the returned error for diagnostics.
func dockerExec(args ...string) error {
	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
