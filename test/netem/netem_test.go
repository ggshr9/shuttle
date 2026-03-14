package netem

import (
	"testing"
	"time"
)

// --- Preset value tests ---

func TestHighLatencyPreset(t *testing.T) {
	imp := HighLatency()
	if imp.Delay != 200*time.Millisecond {
		t.Errorf("Delay = %v, want 200ms", imp.Delay)
	}
	if imp.Jitter != 20*time.Millisecond {
		t.Errorf("Jitter = %v, want 20ms", imp.Jitter)
	}
	if imp.Loss != 0 {
		t.Errorf("Loss = %v, want 0", imp.Loss)
	}
	if imp.Bandwidth != "" {
		t.Errorf("Bandwidth = %q, want empty", imp.Bandwidth)
	}
}

func TestPacketLossPreset(t *testing.T) {
	imp := PacketLoss(15.5)
	if imp.Loss != 15.5 {
		t.Errorf("Loss = %v, want 15.5", imp.Loss)
	}
	if imp.Delay != 0 {
		t.Errorf("Delay = %v, want 0", imp.Delay)
	}
}

func TestPacketLossZero(t *testing.T) {
	imp := PacketLoss(0)
	if imp.Loss != 0 {
		t.Errorf("Loss = %v, want 0", imp.Loss)
	}
}

func TestSatellitePreset(t *testing.T) {
	imp := Satellite()
	if imp.Delay != 500*time.Millisecond {
		t.Errorf("Delay = %v, want 500ms", imp.Delay)
	}
	if imp.Jitter != 50*time.Millisecond {
		t.Errorf("Jitter = %v, want 50ms", imp.Jitter)
	}
	if imp.Loss != 5 {
		t.Errorf("Loss = %v, want 5", imp.Loss)
	}
}

func TestGFWSimulationPreset(t *testing.T) {
	imp := GFWSimulation()
	if imp.Delay != 50*time.Millisecond {
		t.Errorf("Delay = %v, want 50ms", imp.Delay)
	}
	if imp.Loss != 10 {
		t.Errorf("Loss = %v, want 10", imp.Loss)
	}
	if imp.Reorder != 2 {
		t.Errorf("Reorder = %v, want 2", imp.Reorder)
	}
	if imp.Jitter != 0 {
		t.Errorf("Jitter = %v, want 0", imp.Jitter)
	}
}

func TestSlowLinkPreset(t *testing.T) {
	imp := SlowLink()
	if imp.Delay != 100*time.Millisecond {
		t.Errorf("Delay = %v, want 100ms", imp.Delay)
	}
	if imp.Bandwidth != "1mbit" {
		t.Errorf("Bandwidth = %q, want %q", imp.Bandwidth, "1mbit")
	}
}

func TestPristinePreset(t *testing.T) {
	imp := Pristine()
	if imp.Delay != 0 || imp.Jitter != 0 || imp.Loss != 0 ||
		imp.Bandwidth != "" || imp.Reorder != 0 || imp.Duplicate != 0 || imp.Corrupt != 0 {
		t.Errorf("Pristine should be zero value, got %+v", imp)
	}
}

// --- buildNetemArgs tests ---

func TestBuildNetemArgsDelayOnly(t *testing.T) {
	imp := Impairment{Delay: 200 * time.Millisecond}
	args := buildNetemArgs(imp)

	assertArgs(t, args, []string{"delay", "200ms"})
}

func TestBuildNetemArgsDelayWithJitter(t *testing.T) {
	imp := Impairment{Delay: 200 * time.Millisecond, Jitter: 20 * time.Millisecond}
	args := buildNetemArgs(imp)

	assertArgs(t, args, []string{"delay", "200ms", "20ms"})
}

func TestBuildNetemArgsJitterWithoutDelay(t *testing.T) {
	// Jitter without delay should produce no delay/jitter args
	imp := Impairment{Jitter: 20 * time.Millisecond}
	args := buildNetemArgs(imp)

	for _, a := range args {
		if a == "delay" {
			t.Fatal("jitter without delay should not produce delay arg")
		}
	}
}

func TestBuildNetemArgsLossOnly(t *testing.T) {
	imp := Impairment{Loss: 10}
	args := buildNetemArgs(imp)

	assertArgs(t, args, []string{"loss", "10.00%"})
}

func TestBuildNetemArgsReorderOnly(t *testing.T) {
	imp := Impairment{Reorder: 5.5}
	args := buildNetemArgs(imp)

	assertArgs(t, args, []string{"reorder", "5.50%"})
}

func TestBuildNetemArgsDuplicateOnly(t *testing.T) {
	imp := Impairment{Duplicate: 3.0}
	args := buildNetemArgs(imp)

	assertArgs(t, args, []string{"duplicate", "3.00%"})
}

func TestBuildNetemArgsCorruptOnly(t *testing.T) {
	imp := Impairment{Corrupt: 1.5}
	args := buildNetemArgs(imp)

	assertArgs(t, args, []string{"corrupt", "1.50%"})
}

func TestBuildNetemArgsEmpty(t *testing.T) {
	imp := Impairment{}
	args := buildNetemArgs(imp)

	if len(args) != 0 {
		t.Fatalf("expected no args for zero impairment, got %v", args)
	}
}

func TestBuildNetemArgsBandwidthNotIncluded(t *testing.T) {
	// Bandwidth is handled separately by applyBandwidth, not buildNetemArgs
	imp := Impairment{Bandwidth: "10mbit"}
	args := buildNetemArgs(imp)

	for _, a := range args {
		if a == "rate" || a == "10mbit" || a == "tbf" {
			t.Fatalf("bandwidth should not appear in netem args: %v", args)
		}
	}
}

func TestBuildNetemArgsAllFields(t *testing.T) {
	imp := Impairment{
		Delay:     100 * time.Millisecond,
		Jitter:    10 * time.Millisecond,
		Loss:      5,
		Reorder:   2,
		Duplicate: 1,
		Corrupt:   0.5,
	}
	args := buildNetemArgs(imp)

	// Should contain all expected fields
	expected := map[string]bool{
		"delay":     false,
		"loss":      false,
		"reorder":   false,
		"duplicate": false,
		"corrupt":   false,
	}

	for _, a := range args {
		if _, ok := expected[a]; ok {
			expected[a] = true
		}
	}

	for k, v := range expected {
		if !v {
			t.Errorf("expected %q in args, not found. args: %v", k, args)
		}
	}
}

func TestBuildNetemArgsSatellitePreset(t *testing.T) {
	imp := Satellite()
	args := buildNetemArgs(imp)

	// Should have delay 500ms 50ms loss 5.00%
	assertContains(t, args, "delay")
	assertContains(t, args, "500ms")
	assertContains(t, args, "50ms")
	assertContains(t, args, "loss")
	assertContains(t, args, "5.00%")
}

func TestBuildNetemArgsGFWPreset(t *testing.T) {
	imp := GFWSimulation()
	args := buildNetemArgs(imp)

	assertContains(t, args, "delay")
	assertContains(t, args, "50ms")
	assertContains(t, args, "loss")
	assertContains(t, args, "10.00%")
	assertContains(t, args, "reorder")
	assertContains(t, args, "2.00%")
}

// --- fmtDuration tests ---

func TestFmtDurationMilliseconds(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{100 * time.Millisecond, "100ms"},
		{200 * time.Millisecond, "200ms"},
		{1 * time.Second, "1000ms"},
		{50 * time.Millisecond, "50ms"},
		{1 * time.Millisecond, "1ms"},
	}

	for _, tt := range tests {
		got := fmtDuration(tt.d)
		if got != tt.want {
			t.Errorf("fmtDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestFmtDurationMicroseconds(t *testing.T) {
	d := 500 * time.Microsecond
	got := fmtDuration(d)
	if got != "500us" {
		t.Errorf("fmtDuration(%v) = %q, want %q", d, got, "500us")
	}
}

// --- Constants tests ---

func TestDefaultConstants(t *testing.T) {
	if DefaultRouter != "shuttle-router" {
		t.Errorf("DefaultRouter = %q, want %q", DefaultRouter, "shuttle-router")
	}
	if DefaultIface != "eth0" {
		t.Errorf("DefaultIface = %q, want %q", DefaultIface, "eth0")
	}
}

// --- Impairment struct tests ---

func TestImpairmentZeroValue(t *testing.T) {
	var imp Impairment
	args := buildNetemArgs(imp)
	if len(args) != 0 {
		t.Fatalf("zero-value Impairment should produce no args, got %v", args)
	}
}

func TestImpairmentFieldIndependence(t *testing.T) {
	// Each field should independently produce its own args
	fields := []struct {
		name string
		imp  Impairment
		key  string
	}{
		{"Delay", Impairment{Delay: 1 * time.Millisecond}, "delay"},
		{"Loss", Impairment{Loss: 1}, "loss"},
		{"Reorder", Impairment{Reorder: 1}, "reorder"},
		{"Duplicate", Impairment{Duplicate: 1}, "duplicate"},
		{"Corrupt", Impairment{Corrupt: 1}, "corrupt"},
	}

	for _, f := range fields {
		args := buildNetemArgs(f.imp)
		if len(args) == 0 {
			t.Errorf("%s: expected args, got none", f.name)
			continue
		}
		if args[0] != f.key {
			t.Errorf("%s: first arg = %q, want %q", f.name, args[0], f.key)
		}
	}
}

// --- Helper functions ---

func assertArgs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("args length = %d, want %d\ngot:  %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q\nfull: %v", i, got[i], want[i], got)
		}
	}
}

func assertContains(t *testing.T, args []string, want string) {
	t.Helper()
	for _, a := range args {
		if a == want {
			return
		}
	}
	t.Errorf("args %v does not contain %q", args, want)
}
