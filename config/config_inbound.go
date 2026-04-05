package config

import "encoding/json"

// InboundConfig defines a pluggable inbound listener.
type InboundConfig struct {
	Tag     string          `yaml:"tag" json:"tag"`
	Type    string          `yaml:"type" json:"type"`
	Listen  string          `yaml:"listen,omitempty" json:"listen,omitempty"`
	Options json.RawMessage `yaml:"options,omitempty" json:"options,omitempty"`
}

// OutboundConfig defines a pluggable outbound dialer.
type OutboundConfig struct {
	Tag     string          `yaml:"tag" json:"tag"`
	Type    string          `yaml:"type" json:"type"`
	Options json.RawMessage `yaml:"options,omitempty" json:"options,omitempty"`
}
