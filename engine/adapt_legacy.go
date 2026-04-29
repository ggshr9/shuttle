package engine

import (
	"encoding/json"

	"github.com/ggshr9/shuttle/config"
)

// adaptLegacyConfig converts legacy proxy.* config fields into equivalent
// Inbounds[] entries so that old YAML configs work through the unified
// inbound path. If cfg.Inbounds is already non-empty, the function is a
// no-op (explicit config wins).
func adaptLegacyConfig(cfg *config.ClientConfig) {
	if len(cfg.Inbounds) > 0 {
		return
	}

	if cfg.Proxy.SOCKS5.Enabled {
		opts, _ := json.Marshal(map[string]interface{}{
			"listen": cfg.Proxy.SOCKS5.Listen,
		})
		cfg.Inbounds = append(cfg.Inbounds, config.InboundConfig{
			Tag:     "socks5",
			Type:    "socks5",
			Options: opts,
		})
	}

	if cfg.Proxy.HTTP.Enabled {
		opts, _ := json.Marshal(map[string]interface{}{
			"listen": cfg.Proxy.HTTP.Listen,
		})
		cfg.Inbounds = append(cfg.Inbounds, config.InboundConfig{
			Tag:     "http",
			Type:    "http",
			Options: opts,
		})
	}

	if cfg.Proxy.TUN.Enabled {
		t := cfg.Proxy.TUN
		tunOpts := map[string]interface{}{
			"device_name": t.DeviceName,
			"cidr":        t.CIDR,
			"mtu":         t.MTU,
			"auto_route":  t.AutoRoute,
			"tun_fd":      t.TunFD,
		}
		if t.IPv6CIDR != "" {
			tunOpts["ipv6_cidr"] = t.IPv6CIDR
		}
		if t.PerAppMode != "" {
			tunOpts["per_app_mode"] = t.PerAppMode
		}
		if len(t.AppList) > 0 {
			tunOpts["app_list"] = t.AppList
		}
		opts, _ := json.Marshal(tunOpts)
		cfg.Inbounds = append(cfg.Inbounds, config.InboundConfig{
			Tag:     "tun",
			Type:    "tun",
			Options: opts,
		})
	}
}
