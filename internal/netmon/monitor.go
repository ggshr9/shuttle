package netmon

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"
	"unicode"
)

// NetworkType represents the type of network interface.
type NetworkType int

const (
	// NetworkUnknown is the default when the interface type cannot be determined.
	NetworkUnknown NetworkType = iota
	// NetworkWiFi represents a wireless LAN interface.
	NetworkWiFi
	// NetworkCellular represents a mobile/cellular data interface.
	NetworkCellular
	// NetworkEthernet represents a wired Ethernet interface.
	NetworkEthernet
)

// String returns a human-readable name for the network type.
func (n NetworkType) String() string {
	switch n {
	case NetworkWiFi:
		return "wifi"
	case NetworkCellular:
		return "cellular"
	case NetworkEthernet:
		return "ethernet"
	default:
		return "unknown"
	}
}

// ClassifyInterface determines the network type from an interface name.
//
// WiFi: names starting with "wlan", "wl", "en0", "en1" (macOS WiFi), "Wi-Fi"
// Cellular: "rmnet", "wwan", "pdp_ip", "ccmni", "rev_rmnet"
// Ethernet: "eth", "en" (followed by number >1 on macOS), "enp", "eno"
// Unknown: anything else
func ClassifyInterface(name string) NetworkType {
	lower := strings.ToLower(name)

	// WiFi patterns
	if strings.HasPrefix(lower, "wlan") || lower == "wi-fi" {
		return NetworkWiFi
	}
	// "wl" prefix but not "wlan" (already handled above)
	if strings.HasPrefix(lower, "wl") {
		return NetworkWiFi
	}
	// macOS WiFi: en0, en1
	if lower == "en0" || lower == "en1" {
		return NetworkWiFi
	}

	// Cellular patterns
	cellularPrefixes := []string{"rmnet", "wwan", "pdp_ip", "ccmni", "rev_rmnet"}
	for _, prefix := range cellularPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return NetworkCellular
		}
	}

	// Ethernet patterns
	if strings.HasPrefix(lower, "eth") {
		return NetworkEthernet
	}
	if strings.HasPrefix(lower, "enp") || strings.HasPrefix(lower, "eno") {
		return NetworkEthernet
	}
	// macOS: "en" followed by digit > 1 (en2, en3, ...)
	if strings.HasPrefix(lower, "en") && len(lower) > 2 {
		suffix := lower[2:]
		if isDigits(suffix) {
			// en0 and en1 already matched WiFi above, so any remaining enN is Ethernet
			return NetworkEthernet
		}
	}

	return NetworkUnknown
}

// isDigits reports whether s consists entirely of ASCII digits.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// Callback is called when a network change is detected.
type Callback func()

// TypeCallback is called when a network change is detected, with the new network type.
type TypeCallback func(NetworkType)

// Monitor watches for network interface changes and calls registered callbacks.
type Monitor struct {
	mu            sync.Mutex
	callbacks     []Callback
	typeCallbacks []TypeCallback
	interval      time.Duration
	cancel        context.CancelFunc
	lastAddrs     string // cached address snapshot for change detection
}

// New creates a network monitor with the given poll interval.
func New(interval time.Duration) *Monitor {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &Monitor{
		interval: interval,
	}
}

// OnChange registers a callback to be invoked on network change.
func (m *Monitor) OnChange(cb Callback) {
	m.mu.Lock()
	m.callbacks = append(m.callbacks, cb)
	m.mu.Unlock()
}

// OnChangeWithType registers a callback that receives the current network type
// when a network change is detected.
func (m *Monitor) OnChangeWithType(cb TypeCallback) {
	m.mu.Lock()
	m.typeCallbacks = append(m.typeCallbacks, cb)
	m.mu.Unlock()
}

// CurrentType returns the best current network type by inspecting all active
// interfaces. Priority: WiFi > Cellular > Ethernet > Unknown.
func (m *Monitor) CurrentType() NetworkType {
	return detectCurrentType()
}

// detectCurrentType inspects system interfaces and returns the best network type.
func detectCurrentType() NetworkType {
	ifaces, err := net.Interfaces()
	if err != nil {
		return NetworkUnknown
	}

	best := NetworkUnknown
	for _, iface := range ifaces {
		// Skip down or loopback interfaces.
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		// Skip interfaces with no addresses.
		addrs, err := iface.Addrs()
		if err != nil || len(addrs) == 0 {
			continue
		}

		t := ClassifyInterface(iface.Name)
		if networkTypePriority(t) > networkTypePriority(best) {
			best = t
		}
	}
	return best
}

// networkTypePriority returns a priority value for network type ordering.
// Higher is better: WiFi(3) > Cellular(2) > Ethernet(1) > Unknown(0).
func networkTypePriority(t NetworkType) int {
	switch t {
	case NetworkWiFi:
		return 3
	case NetworkCellular:
		return 2
	case NetworkEthernet:
		return 1
	default:
		return 0
	}
}

// Start begins monitoring in a background goroutine.
func (m *Monitor) Start(ctx context.Context) {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
	}
	ctx, m.cancel = context.WithCancel(ctx)
	m.lastAddrs = getAddressSnapshot()
	m.mu.Unlock()

	go m.pollLoop(ctx)
}

// Stop stops the monitor.
func (m *Monitor) Stop() {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.mu.Unlock()
}

func (m *Monitor) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current := getAddressSnapshot()
			m.mu.Lock()
			changed := current != m.lastAddrs
			if changed {
				m.lastAddrs = current
			}
			callbacks := make([]Callback, len(m.callbacks))
			copy(callbacks, m.callbacks)
			typeCallbacks := make([]TypeCallback, len(m.typeCallbacks))
			copy(typeCallbacks, m.typeCallbacks)
			m.mu.Unlock()

			if changed {
				netType := detectCurrentType()
				for _, cb := range callbacks {
					cb()
				}
				for _, cb := range typeCallbacks {
					cb(netType)
				}
			}
		}
	}
}

// getAddressSnapshot returns a string representation of all network interface addresses.
// Changes to this string indicate a network change.
func getAddressSnapshot() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	var s string
	for _, addr := range addrs {
		s += addr.String() + ";"
	}
	return s
}
