package config

import (
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config version constants.
const (
	CurrentClientConfigVersion = 1
	CurrentServerConfigVersion = 1
)

// ClientConfig is the top-level client configuration.
type ClientConfig struct {
	Version       int                  `yaml:"version" json:"version"`
	Server        ServerEndpoint       `yaml:"server" json:"server"`
	Servers       []ServerEndpoint     `yaml:"servers,omitempty" json:"servers,omitempty"` // saved server list
	Subscriptions []SubscriptionConfig `yaml:"subscriptions,omitempty" json:"subscriptions,omitempty"`
	Transport     TransportConfig      `yaml:"transport" json:"transport"`
	Proxy         ProxyConfig          `yaml:"proxy" json:"proxy"`
	Routing       RoutingConfig        `yaml:"routing" json:"routing"`
	QoS           QoSConfig            `yaml:"qos" json:"qos"`
	Congestion    CongestionConfig     `yaml:"congestion" json:"congestion"`
	Retry         RetryConfig          `yaml:"retry" json:"retry"`
	Mesh          MeshConfig           `yaml:"mesh" json:"mesh"`
	Obfs          ObfsConfig           `yaml:"obfs" json:"obfs"`
	Yamux         YamuxConfig          `yaml:"yamux" json:"yamux"`
	Log           LogConfig            `yaml:"log" json:"log"`
	Inbounds       []InboundConfig       `yaml:"inbounds,omitempty" json:"inbounds,omitempty"`
	Outbounds      []OutboundConfig      `yaml:"outbounds,omitempty" json:"outbounds,omitempty"`
	ProxyProviders []ProxyProviderConfig `yaml:"proxy_providers,omitempty" json:"proxy_providers,omitempty"`
	RuleProviders  []RuleProviderConfig  `yaml:"rule_providers,omitempty" json:"rule_providers,omitempty"`
}

// SubscriptionConfig represents a subscription source.
type SubscriptionConfig struct {
	ID   string `yaml:"id" json:"id"`
	Name string `yaml:"name" json:"name"`
	URL  string `yaml:"url" json:"url"`
}

// RetryConfig configures connection retry with exponential backoff.
type RetryConfig struct {
	MaxAttempts    int    `yaml:"max_attempts" json:"max_attempts"`
	InitialBackoff string `yaml:"initial_backoff" json:"initial_backoff"` // duration string, e.g. "100ms"
	MaxBackoff     string `yaml:"max_backoff" json:"max_backoff"`         // duration string, e.g. "5s"
}

// ServerEndpoint defines a remote server.
type ServerEndpoint struct {
	Addr     string `yaml:"addr" json:"addr"`
	Name     string `yaml:"name" json:"name"`
	Password string `yaml:"password" json:"password"`
	SNI      string `yaml:"sni" json:"sni"`
}

// LogConfig configures logging.
type LogConfig struct {
	Level  string `yaml:"level" json:"level"`   // "debug", "info", "warn", "error"
	Format string `yaml:"format" json:"format"` // "text" (default) or "json"
	Output string `yaml:"output" json:"output"` // "stdout", "stderr", or file path
}

// LoadClientConfig loads client config from a YAML file.
// If the keystore is initialized, any "ENC:"-prefixed sensitive fields
// are automatically decrypted. Plaintext values pass through unchanged
// for backward compatibility.
func LoadClientConfig(path string) (*ClientConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg ClientConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	applyClientDefaults(&cfg)
	if cfg.Version == 0 {
		cfg.Version = CurrentClientConfigVersion
	}
	// Auto-decrypt sensitive fields if keystore is available.
	if key, err := GetKey(); err == nil {
		if err := decryptSensitiveFields(&cfg, key); err != nil {
			return nil, fmt.Errorf("decrypt config: %w", err)
		}
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}
	return &cfg, nil
}

// SaveClientConfig writes a client config to a YAML file.
// If the keystore is initialized, sensitive fields are encrypted before
// writing. The in-memory config is not modified.
func SaveClientConfig(path string, cfg *ClientConfig) error {
	// Work on a copy so we don't mutate the caller's config.
	cp := cfg.DeepCopy()

	// Encrypt sensitive fields if keystore is available.
	if key, err := GetKey(); err == nil {
		if err := encryptSensitiveFields(cp, key); err != nil {
			return fmt.Errorf("encrypt config: %w", err)
		}
	}

	data, err := yaml.Marshal(cp)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}

// LoadServerConfig loads server config from a YAML file.
// If the keystore is initialized, any "ENC:"-prefixed sensitive fields
// are automatically decrypted. Plaintext values pass through unchanged
// for backward compatibility.
func LoadServerConfig(path string) (*ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg ServerConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	applyServerDefaults(&cfg)
	if cfg.Version == 0 {
		cfg.Version = CurrentServerConfigVersion
	}
	// Auto-decrypt sensitive fields if keystore is available.
	if key, err := GetKey(); err == nil {
		if err := decryptServerSensitiveFields(&cfg, key); err != nil {
			return nil, fmt.Errorf("decrypt config: %w", err)
		}
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}
	return &cfg, nil
}

// SaveServerConfig writes a server config to a YAML file.
// If the keystore is initialized, sensitive fields (password, private key,
// admin token, cluster secret) are encrypted before writing. The in-memory
// config is not modified.
func SaveServerConfig(path string, cfg *ServerConfig) error {
	// Deep copy to avoid mutating the caller's config.
	cp := *cfg
	// Copy slices that may be modified.
	if cfg.Admin.Users != nil {
		cp.Admin.Users = make([]User, len(cfg.Admin.Users))
		copy(cp.Admin.Users, cfg.Admin.Users)
	}

	// Encrypt sensitive fields if keystore is available.
	if key, err := GetKey(); err == nil {
		if err := encryptServerSensitiveFields(&cp, key); err != nil {
			return fmt.Errorf("encrypt config: %w", err)
		}
	}

	data, err := yaml.Marshal(&cp)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}

// DeepCopy returns a fully independent copy of the config.
func (c *ClientConfig) DeepCopy() *ClientConfig {
	cp := *c
	// Copy slices that contain reference types
	if c.Servers != nil {
		cp.Servers = make([]ServerEndpoint, len(c.Servers))
		copy(cp.Servers, c.Servers)
	}
	if c.Subscriptions != nil {
		cp.Subscriptions = make([]SubscriptionConfig, len(c.Subscriptions))
		copy(cp.Subscriptions, c.Subscriptions)
	}
	if c.Routing.Rules != nil {
		cp.Routing.Rules = make([]RouteRule, len(c.Routing.Rules))
		for i, r := range c.Routing.Rules {
			cp.Routing.Rules[i] = r
			if r.Process != nil {
				cp.Routing.Rules[i].Process = make([]string, len(r.Process))
				copy(cp.Routing.Rules[i].Process, r.Process)
			}
			if r.IPCIDR != nil {
				cp.Routing.Rules[i].IPCIDR = make([]string, len(r.IPCIDR))
				copy(cp.Routing.Rules[i].IPCIDR, r.IPCIDR)
			}
		}
	}
	if c.Proxy.TUN.AppList != nil {
		cp.Proxy.TUN.AppList = make([]string, len(c.Proxy.TUN.AppList))
		copy(cp.Proxy.TUN.AppList, c.Proxy.TUN.AppList)
	}
	if c.Mesh.P2P.STUNServers != nil {
		cp.Mesh.P2P.STUNServers = make([]string, len(c.Mesh.P2P.STUNServers))
		copy(cp.Mesh.P2P.STUNServers, c.Mesh.P2P.STUNServers)
	}
	if c.Transport.WebRTC.STUNServers != nil {
		cp.Transport.WebRTC.STUNServers = make([]string, len(c.Transport.WebRTC.STUNServers))
		copy(cp.Transport.WebRTC.STUNServers, c.Transport.WebRTC.STUNServers)
	}
	if c.Transport.WebRTC.TURNServers != nil {
		cp.Transport.WebRTC.TURNServers = make([]string, len(c.Transport.WebRTC.TURNServers))
		copy(cp.Transport.WebRTC.TURNServers, c.Transport.WebRTC.TURNServers)
	}
	if c.Mesh.SplitRoutes != nil {
		cp.Mesh.SplitRoutes = make([]SplitRoute, len(c.Mesh.SplitRoutes))
		copy(cp.Mesh.SplitRoutes, c.Mesh.SplitRoutes)
	}
	if c.QoS.Rules != nil {
		cp.QoS.Rules = make([]QoSRule, len(c.QoS.Rules))
		for i, r := range c.QoS.Rules {
			cp.QoS.Rules[i] = r
			if r.Ports != nil {
				cp.QoS.Rules[i].Ports = make([]uint16, len(r.Ports))
				copy(cp.QoS.Rules[i].Ports, r.Ports)
			}
			if r.Domains != nil {
				cp.QoS.Rules[i].Domains = make([]string, len(r.Domains))
				copy(cp.QoS.Rules[i].Domains, r.Domains)
			}
			if r.Process != nil {
				cp.QoS.Rules[i].Process = make([]string, len(r.Process))
				copy(cp.QoS.Rules[i].Process, r.Process)
			}
		}
	}
	if c.Inbounds != nil {
		cp.Inbounds = make([]InboundConfig, len(c.Inbounds))
		for i, ib := range c.Inbounds {
			cp.Inbounds[i] = ib
			if ib.Options != nil {
				cp.Inbounds[i].Options = make(json.RawMessage, len(ib.Options))
				copy(cp.Inbounds[i].Options, ib.Options)
			}
		}
	}
	if c.Outbounds != nil {
		cp.Outbounds = make([]OutboundConfig, len(c.Outbounds))
		for i, ob := range c.Outbounds {
			cp.Outbounds[i] = ob
			if ob.Options != nil {
				cp.Outbounds[i].Options = make(json.RawMessage, len(ob.Options))
				copy(cp.Outbounds[i].Options, ob.Options)
			}
		}
	}
	return &cp
}
