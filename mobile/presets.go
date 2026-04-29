// Package mobile provides gomobile-compatible bindings for the shuttle engine.
package mobile

import (
	"encoding/json"

	"github.com/ggshr9/shuttle/config"
)

// NetworkPreset defines a named configuration profile optimized for a specific
// network environment. Native apps can apply a preset to tune performance
// without manual configuration.
type NetworkPreset struct {
	Name        string // Human-readable name (e.g., "WiFi", "LTE", "5G")
	Description string // Short description of the preset

	// Transport preference for this network type
	PreferredTransport string // "auto", "h3", "reality", "cdn"

	// Congestion control tuning
	CongestionMode string // "adaptive", "bbr", "brutal"

	// Keepalive intervals (shorter on cellular to detect stale connections)
	KeepaliveIntervalSec int

	// Reconnect tuning
	MaxRetries   int
	BaseDelayMs  int
	MaxDelayMs   int

	// Obfuscation - more aggressive padding on cellular
	PaddingEnabled bool
}

// Preset constants for well-known network environments.
var (
	// PresetWiFi is optimized for stable, high-bandwidth WiFi connections.
	PresetWiFi = &NetworkPreset{
		Name:                 "wifi",
		Description:          "High-bandwidth WiFi — relaxed keepalive, BBR congestion",
		PreferredTransport:   "auto",
		CongestionMode:       "bbr",
		KeepaliveIntervalSec: 30,
		MaxRetries:           3,
		BaseDelayMs:          1000,
		MaxDelayMs:           30000,
		PaddingEnabled:       false,
	}

	// PresetLTE is optimized for stable 4G/LTE cellular connections.
	PresetLTE = &NetworkPreset{
		Name:                 "lte",
		Description:          "4G/LTE — moderate keepalive, adaptive congestion",
		PreferredTransport:   "auto",
		CongestionMode:       "adaptive",
		KeepaliveIntervalSec: 15,
		MaxRetries:           5,
		BaseDelayMs:          500,
		MaxDelayMs:           15000,
		PaddingEnabled:       true,
	}

	// Preset5G is optimized for 5G with high bandwidth but variable latency.
	Preset5G = &NetworkPreset{
		Name:                 "5g",
		Description:          "5G — aggressive BBR, fast reconnect",
		PreferredTransport:   "h3",
		CongestionMode:       "bbr",
		KeepaliveIntervalSec: 20,
		MaxRetries:           3,
		BaseDelayMs:          300,
		MaxDelayMs:           10000,
		PaddingEnabled:       false,
	}

	// PresetSlowCellular is for degraded 3G/EDGE networks.
	PresetSlowCellular = &NetworkPreset{
		Name:                 "slow_cellular",
		Description:          "3G/EDGE — conservative settings, CDN transport",
		PreferredTransport:   "cdn",
		CongestionMode:       "adaptive",
		KeepaliveIntervalSec: 10,
		MaxRetries:           7,
		BaseDelayMs:          1000,
		MaxDelayMs:           60000,
		PaddingEnabled:       true,
	}

	// PresetDataSaver minimizes bandwidth usage.
	PresetDataSaver = &NetworkPreset{
		Name:                 "data_saver",
		Description:          "Data saver — minimal overhead, no padding, long keepalive",
		PreferredTransport:   "cdn",
		CongestionMode:       "adaptive",
		KeepaliveIntervalSec: 60,
		MaxRetries:           3,
		BaseDelayMs:          2000,
		MaxDelayMs:           60000,
		PaddingEnabled:       false,
	}
)

// knownPresets maps preset names to their definitions.
var knownPresets = map[string]*NetworkPreset{
	"wifi":          PresetWiFi,
	"lte":           PresetLTE,
	"5g":            Preset5G,
	"slow_cellular": PresetSlowCellular,
	"data_saver":    PresetDataSaver,
}

// GetPreset returns a network preset by name as JSON.
// Returns empty string if the preset is not found.
// Known presets: "wifi", "lte", "5g", "slow_cellular", "data_saver"
func GetPreset(name string) string {
	p, ok := knownPresets[name]
	if !ok {
		return ""
	}
	data, _ := json.Marshal(p)
	return string(data)
}

// ListPresets returns a JSON array of all available preset names and descriptions.
func ListPresets() string {
	type presetInfo struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	presets := []*NetworkPreset{PresetWiFi, PresetLTE, Preset5G, PresetSlowCellular, PresetDataSaver}
	list := make([]presetInfo, 0, len(presets))
	for _, p := range presets {
		list = append(list, presetInfo{Name: p.Name, Description: p.Description})
	}
	data, _ := json.Marshal(list)
	return string(data)
}

// ApplyPreset applies a named preset to a JSON config and returns the modified config.
// This modifies transport, congestion, and obfuscation settings based on the preset.
// Returns the original config unchanged if the preset is not found.
func ApplyPreset(configJSON string, presetName string) string {
	preset, ok := knownPresets[presetName]
	if !ok {
		return configJSON
	}

	var cfg config.ClientConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return configJSON
	}

	applyPresetToConfig(&cfg, preset)

	data, err := json.Marshal(&cfg)
	if err != nil {
		return configJSON
	}
	return string(data)
}

// applyPresetToConfig modifies a config according to a preset.
func applyPresetToConfig(cfg *config.ClientConfig, p *NetworkPreset) {
	if p.PreferredTransport != "" {
		cfg.Transport.Preferred = p.PreferredTransport
	}
	if p.CongestionMode != "" {
		cfg.Congestion.Mode = p.CongestionMode
	}
	cfg.Obfs.PaddingEnabled = p.PaddingEnabled
}
