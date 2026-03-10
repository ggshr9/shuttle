package geodata

import (
	"testing"
)

func TestBuiltinPresets(t *testing.T) {
	presets := BuiltinPresets()
	if len(presets) < 3 {
		t.Fatalf("expected at least 3 presets, got %d", len(presets))
	}

	ids := make(map[string]bool)
	for _, p := range presets {
		if p.ID == "" {
			t.Fatal("preset has empty ID")
		}
		if p.Name == "" {
			t.Fatalf("preset %s has empty name", p.ID)
		}
		if ids[p.ID] {
			t.Fatalf("duplicate preset ID: %s", p.ID)
		}
		ids[p.ID] = true
	}
}

func TestPresetByID(t *testing.T) {
	p := PresetByID("loyalsoldier")
	if p == nil {
		t.Fatal("expected loyalsoldier preset")
	}
	if p.DirectList == "" {
		t.Fatal("loyalsoldier preset missing DirectList URL")
	}
	if p.CNCidr == "" {
		t.Fatal("loyalsoldier preset missing CNCidr URL")
	}
}

func TestPresetByIDV2fly(t *testing.T) {
	p := PresetByID("v2fly")
	if p == nil {
		t.Fatal("expected v2fly preset")
	}
	if p.DirectList == "" {
		t.Fatal("v2fly preset missing DirectList URL")
	}
}

func TestPresetByIDCustom(t *testing.T) {
	p := PresetByID("custom")
	if p == nil {
		t.Fatal("expected custom preset")
	}
	// Custom preset has empty URLs (user fills them in)
	if p.DirectList != "" {
		t.Fatal("custom preset should have empty DirectList")
	}
}

func TestPresetByIDNotFound(t *testing.T) {
	p := PresetByID("nonexistent")
	if p != nil {
		t.Fatal("expected nil for nonexistent preset")
	}
}
