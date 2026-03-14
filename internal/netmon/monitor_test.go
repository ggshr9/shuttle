package netmon

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestMonitorStartStop(t *testing.T) {
	m := New(100 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.Start(ctx)
	// Should not panic or block.
	time.Sleep(50 * time.Millisecond)
	m.Stop()
}

func TestMonitorOnChange(t *testing.T) {
	m := New(time.Second)

	var called int32
	m.OnChange(func() {
		atomic.AddInt32(&called, 1)
	})

	m.mu.Lock()
	if len(m.callbacks) != 1 {
		t.Fatalf("expected 1 callback, got %d", len(m.callbacks))
	}
	m.mu.Unlock()
}

func TestMonitorDoubleStart(t *testing.T) {
	m := New(100 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.Start(ctx)
	// Second start should cancel the first and not panic.
	m.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	m.Stop()
}

func TestGetAddressSnapshot(t *testing.T) {
	snap := getAddressSnapshot()
	if snap == "" {
		t.Skip("no network interfaces available")
	}
	// Should contain at least one semicolon (separator).
	found := false
	for _, c := range snap {
		if c == ';' {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("snapshot missing separator: %q", snap)
	}
}

func TestMonitorDetectsChange(t *testing.T) {
	m := New(50 * time.Millisecond)

	var notified atomic.Int32
	m.OnChange(func() {
		notified.Add(1)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.Start(ctx)

	// Simulate a network change by modifying lastAddrs to something different.
	m.mu.Lock()
	m.lastAddrs = "fake-old-snapshot"
	m.mu.Unlock()

	// Wait for the poll loop to detect the change.
	deadline := time.After(2 * time.Second)
	for {
		if notified.Load() > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for change callback")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	m.Stop()
}

// ---------------------------------------------------------------------------
// ClassifyInterface tests
// ---------------------------------------------------------------------------

func TestClassifyInterfaceWiFi(t *testing.T) {
	tests := []struct {
		name string
		want NetworkType
	}{
		{"wlan0", NetworkWiFi},
		{"wlan1", NetworkWiFi},
		{"wlp2s0", NetworkWiFi},
		{"en0", NetworkWiFi},
		{"en1", NetworkWiFi},
		{"Wi-Fi", NetworkWiFi},
		{"wi-fi", NetworkWiFi},
	}
	for _, tt := range tests {
		got := ClassifyInterface(tt.name)
		if got != tt.want {
			t.Errorf("ClassifyInterface(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestClassifyInterfaceCellular(t *testing.T) {
	tests := []struct {
		name string
		want NetworkType
	}{
		{"rmnet0", NetworkCellular},
		{"rmnet_data0", NetworkCellular},
		{"wwan0", NetworkCellular},
		{"pdp_ip0", NetworkCellular},
		{"ccmni0", NetworkCellular},
		{"rev_rmnet0", NetworkCellular},
	}
	for _, tt := range tests {
		got := ClassifyInterface(tt.name)
		if got != tt.want {
			t.Errorf("ClassifyInterface(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestClassifyInterfaceEthernet(t *testing.T) {
	tests := []struct {
		name string
		want NetworkType
	}{
		{"eth0", NetworkEthernet},
		{"eth1", NetworkEthernet},
		{"enp0s3", NetworkEthernet},
		{"eno1", NetworkEthernet},
		{"en2", NetworkEthernet},
		{"en10", NetworkEthernet},
	}
	for _, tt := range tests {
		got := ClassifyInterface(tt.name)
		if got != tt.want {
			t.Errorf("ClassifyInterface(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestClassifyInterfaceUnknown(t *testing.T) {
	tests := []struct {
		name string
		want NetworkType
	}{
		{"lo", NetworkUnknown},
		{"tun0", NetworkUnknown},
		{"docker0", NetworkUnknown},
		{"br-1234", NetworkUnknown},
		{"veth123", NetworkUnknown},
	}
	for _, tt := range tests {
		got := ClassifyInterface(tt.name)
		if got != tt.want {
			t.Errorf("ClassifyInterface(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestNetworkTypeString(t *testing.T) {
	tests := []struct {
		nt   NetworkType
		want string
	}{
		{NetworkWiFi, "wifi"},
		{NetworkCellular, "cellular"},
		{NetworkEthernet, "ethernet"},
		{NetworkUnknown, "unknown"},
	}
	for _, tt := range tests {
		got := tt.nt.String()
		if got != tt.want {
			t.Errorf("NetworkType(%d).String() = %q, want %q", tt.nt, got, tt.want)
		}
	}
}

func TestCurrentTypeReturnsValidType(t *testing.T) {
	m := New(time.Second)
	nt := m.CurrentType()
	// We can't predict which type, but it must be one of the valid constants.
	switch nt {
	case NetworkWiFi, NetworkCellular, NetworkEthernet, NetworkUnknown:
		// ok
	default:
		t.Errorf("CurrentType() returned unexpected value: %d", nt)
	}
}

func TestNetworkTypePriority(t *testing.T) {
	// WiFi > Cellular > Ethernet > Unknown
	if networkTypePriority(NetworkWiFi) <= networkTypePriority(NetworkCellular) {
		t.Error("WiFi should have higher priority than Cellular")
	}
	if networkTypePriority(NetworkCellular) <= networkTypePriority(NetworkEthernet) {
		t.Error("Cellular should have higher priority than Ethernet")
	}
	if networkTypePriority(NetworkEthernet) <= networkTypePriority(NetworkUnknown) {
		t.Error("Ethernet should have higher priority than Unknown")
	}
}

func TestOnChangeWithType(t *testing.T) {
	m := New(50 * time.Millisecond)

	var receivedType atomic.Int32
	receivedType.Store(-1)
	m.OnChangeWithType(func(nt NetworkType) {
		receivedType.Store(int32(nt))
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.Start(ctx)

	// Simulate a network change.
	m.mu.Lock()
	m.lastAddrs = "fake-old-snapshot-typed"
	m.mu.Unlock()

	deadline := time.After(2 * time.Second)
	for {
		if receivedType.Load() >= 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for typed change callback")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	m.Stop()

	// Verify we got a valid network type.
	got := NetworkType(receivedType.Load())
	switch got {
	case NetworkWiFi, NetworkCellular, NetworkEthernet, NetworkUnknown:
		// ok
	default:
		t.Errorf("OnChangeWithType received unexpected type: %d", got)
	}
}
