package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// validateHostPort checks that s is a valid host:port pair.
func validateHostPort(s, field string) error {
	if _, _, err := net.SplitHostPort(s); err != nil {
		return fmt.Errorf("invalid %s: %w", field, err)
	}
	return nil
}

// validateURL checks that s is a valid URL with an http or https scheme.
func validateURL(s, field string) error {
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("invalid %s: %w", field, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid %s: scheme must be http or https, got %q", field, u.Scheme)
	}
	return nil
}

// validateBoundedDuration parses s as a Go duration and rejects values
// outside [minD, maxD] (inclusive). field is the config path used in the
// error message.
func validateBoundedDuration(s, field string, minD, maxD time.Duration) error {
	d, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid %s: %w", field, err)
	}
	if d < minD || d > maxD {
		return fmt.Errorf("invalid %s: %s is out of range [%s, %s]", field, d, minD, maxD)
	}
	return nil
}

// validateCIDR checks that s is a valid CIDR notation.
func validateCIDR(s, field string) error {
	if _, _, err := net.ParseCIDR(s); err != nil {
		return fmt.Errorf("invalid %s: %w", field, err)
	}
	return nil
}

// Validate checks the client config for obviously wrong values.
func (c *ClientConfig) Validate() error {
	switch c.Transport.Preferred {
	case "auto", "h3", "reality", "cdn", "webrtc", "multipath":
	default:
		return fmt.Errorf("invalid transport.preferred: %q", c.Transport.Preferred)
	}
	switch c.Routing.Default {
	case "proxy", "direct":
	default:
		return fmt.Errorf("invalid routing.default: %q", c.Routing.Default)
	}
	if c.Proxy.SOCKS5.Listen != "" {
		if _, _, err := net.SplitHostPort(c.Proxy.SOCKS5.Listen); err != nil {
			return fmt.Errorf("invalid proxy.socks5.listen: %w", err)
		}
	}
	if c.Proxy.HTTP.Listen != "" {
		if _, _, err := net.SplitHostPort(c.Proxy.HTTP.Listen); err != nil {
			return fmt.Errorf("invalid proxy.http.listen: %w", err)
		}
	}
	switch c.Congestion.Mode {
	case "adaptive", "bbr", "brutal", "":
	default:
		return fmt.Errorf("invalid congestion.mode: %q", c.Congestion.Mode)
	}
	if c.Obfs.MaxDelay != "" {
		if err := validateBoundedDuration(c.Obfs.MaxDelay, "obfs.max_delay", 0, 5*time.Second); err != nil {
			return err
		}
	}
	if c.Obfs.MinDelay != "" {
		if err := validateBoundedDuration(c.Obfs.MinDelay, "obfs.min_delay", 0, 5*time.Second); err != nil {
			return err
		}
	}
	if c.Obfs.MinDelay != "" && c.Obfs.MaxDelay != "" {
		minD, _ := time.ParseDuration(c.Obfs.MinDelay)
		maxD, _ := time.ParseDuration(c.Obfs.MaxDelay)
		if minD > maxD {
			return fmt.Errorf("invalid obfs: min_delay (%s) must not exceed max_delay (%s)", minD, maxD)
		}
	}
	for _, sr := range c.Mesh.SplitRoutes {
		if _, _, err := net.ParseCIDR(sr.CIDR); err != nil {
			return fmt.Errorf("invalid mesh split_route CIDR %q: %w", sr.CIDR, err)
		}
		switch sr.Action {
		case "mesh", "direct", "proxy":
		default:
			return fmt.Errorf("invalid mesh split_route action: %q (must be mesh, direct, or proxy)", sr.Action)
		}
	}

	// Multipath mode validation
	if c.Transport.H3.Multipath.Mode != "" {
		switch c.Transport.H3.Multipath.Mode {
		case "failover", "aggregate", "redundant":
		default:
			return fmt.Errorf("invalid transport.h3.multipath.mode: %q (must be failover, aggregate, or redundant)", c.Transport.H3.Multipath.Mode)
		}
	}

	// Transport duration validations
	if c.Transport.PoolIdleTTL != "" {
		if err := validateBoundedDuration(c.Transport.PoolIdleTTL, "transport.pool_idle_ttl", 10*time.Second, 168*time.Hour); err != nil {
			return err
		}
	}
	if c.Transport.KeepaliveInterval != "" {
		if err := validateBoundedDuration(c.Transport.KeepaliveInterval, "transport.keepalive_interval", 1*time.Second, 10*time.Minute); err != nil {
			return err
		}
	}
	if c.Transport.KeepaliveTimeout != "" {
		if err := validateBoundedDuration(c.Transport.KeepaliveTimeout, "transport.keepalive_timeout", 100*time.Millisecond, 60*time.Second); err != nil {
			return err
		}
	}
	if c.Transport.MigrationProbeTimeout != "" {
		if err := validateBoundedDuration(c.Transport.MigrationProbeTimeout, "transport.migration_probe_timeout", 100*time.Millisecond, 60*time.Second); err != nil {
			return err
		}
	}
	if c.Transport.ProbeTimeout != "" {
		if err := validateBoundedDuration(c.Transport.ProbeTimeout, "transport.probe_timeout", 100*time.Millisecond, 60*time.Second); err != nil {
			return err
		}
	}
	if c.Transport.HealthCheckInterval != "" {
		if err := validateBoundedDuration(c.Transport.HealthCheckInterval, "transport.health_check_interval", 1*time.Second, 10*time.Minute); err != nil {
			return err
		}
	}

	// Multipath probe interval validation
	if c.Transport.H3.Multipath.ProbeInterval != "" {
		if err := validateBoundedDuration(c.Transport.H3.Multipath.ProbeInterval, "transport.h3.multipath.probe_interval", 1*time.Second, 10*time.Minute); err != nil {
			return err
		}
	}

	// Retry validation
	if c.Retry.MaxAttempts < 0 {
		return fmt.Errorf("retry.max_attempts must be >= 0")
	}
	if c.Retry.InitialBackoff != "" {
		if err := validateBoundedDuration(c.Retry.InitialBackoff, "retry.initial_backoff", 1*time.Second, 10*time.Minute); err != nil {
			return err
		}
	}
	if c.Retry.MaxBackoff != "" {
		if err := validateBoundedDuration(c.Retry.MaxBackoff, "retry.max_backoff", 1*time.Second, 1*time.Hour); err != nil {
			return err
		}
	}

	// Mesh P2P duration validations
	if c.Mesh.P2P.HolePunchTimeout != "" {
		if err := validateBoundedDuration(c.Mesh.P2P.HolePunchTimeout, "mesh.p2p.hole_punch_timeout", 100*time.Millisecond, 60*time.Second); err != nil {
			return err
		}
	}
	if c.Mesh.P2P.DirectRetryInterval != "" {
		if err := validateBoundedDuration(c.Mesh.P2P.DirectRetryInterval, "mesh.p2p.direct_retry_interval", 1*time.Second, 10*time.Minute); err != nil {
			return err
		}
	}
	if c.Mesh.P2P.KeepAliveInterval != "" {
		if err := validateBoundedDuration(c.Mesh.P2P.KeepAliveInterval, "mesh.p2p.keep_alive_interval", 1*time.Second, 10*time.Minute); err != nil {
			return err
		}
	}

	// Proxy and Rule providers interval validation
	for i, pp := range c.ProxyProviders {
		if pp.Interval != "" {
			if err := validateBoundedDuration(pp.Interval, fmt.Sprintf("proxy_providers[%d].interval", i), 10*time.Second, 168*time.Hour); err != nil {
				return err
			}
		}
		if pp.HealthCheck != nil {
			if pp.HealthCheck.Interval != "" {
				if err := validateBoundedDuration(pp.HealthCheck.Interval,
					fmt.Sprintf("proxy_providers[%d].health_check.interval", i),
					1*time.Second, 10*time.Minute); err != nil {
					return err
				}
			}
			if pp.HealthCheck.Timeout != "" {
				if err := validateBoundedDuration(pp.HealthCheck.Timeout,
					fmt.Sprintf("proxy_providers[%d].health_check.timeout", i),
					100*time.Millisecond, 60*time.Second); err != nil {
					return err
				}
			}
		}
	}
	for i, rp := range c.RuleProviders {
		if rp.Interval != "" {
			if err := validateBoundedDuration(rp.Interval, fmt.Sprintf("rule_providers[%d].interval", i), 10*time.Second, 168*time.Hour); err != nil {
				return err
			}
		}
	}

	// Server.Addr validation
	if c.Server.Addr != "" {
		if err := validateHostPort(c.Server.Addr, "server.addr"); err != nil {
			return err
		}
	}

	// Servers[] validation
	for i, s := range c.Servers {
		if s.Addr != "" {
			if err := validateHostPort(s.Addr, fmt.Sprintf("servers[%d].addr", i)); err != nil {
				return err
			}
		}
	}

	// Subscriptions[].URL validation
	for i, sub := range c.Subscriptions {
		if sub.URL != "" {
			if err := validateURL(sub.URL, fmt.Sprintf("subscriptions[%d].url", i)); err != nil {
				return err
			}
		}
	}

	// Routing.RuleChain validation
	for i := range c.Routing.RuleChain {
		if err := validateRuleChainEntry(i, &c.Routing.RuleChain[i]); err != nil {
			return err
		}
	}

	// Routing.GeoData.UpdateInterval validation
	if c.Routing.GeoData.UpdateInterval != "" {
		if err := validateBoundedDuration(c.Routing.GeoData.UpdateInterval, "routing.geodata.update_interval", 10*time.Second, 168*time.Hour); err != nil {
			return err
		}
	}

	// Routing.DNS.Domestic validation
	if c.Routing.DNS.Domestic != "" {
		switch {
		case strings.Contains(c.Routing.DNS.Domestic, "://"):
			if err := validateURL(c.Routing.DNS.Domestic, "routing.dns.domestic"); err != nil {
				return err
			}
		case strings.Contains(c.Routing.DNS.Domestic, ":"):
			// host:port form
			if err := validateHostPort(c.Routing.DNS.Domestic, "routing.dns.domestic"); err != nil {
				return err
			}
		default:
			// plain IP — validate it parses
			if net.ParseIP(c.Routing.DNS.Domestic) == nil {
				return fmt.Errorf("invalid routing.dns.domestic: %q is not a valid IP address", c.Routing.DNS.Domestic)
			}
		}
	}

	// Routing.DNS.Remote.Server — must be valid DoH URL with https scheme
	if c.Routing.DNS.Remote.Server != "" {
		u, err := url.Parse(c.Routing.DNS.Remote.Server)
		if err != nil {
			return fmt.Errorf("invalid routing.dns.remote.server: %w", err)
		}
		if u.Scheme != "https" {
			return fmt.Errorf("invalid routing.dns.remote.server: scheme must be https, got %q", u.Scheme)
		}
	}

	// Routing.DNS.Remote.Via
	switch c.Routing.DNS.Remote.Via {
	case "proxy", "direct", "":
	default:
		return fmt.Errorf("invalid routing.dns.remote.via: %q (must be \"proxy\", \"direct\", or empty)", c.Routing.DNS.Remote.Via)
	}

	// Transport.CDN validation
	if c.Transport.CDN.Enabled {
		if c.Transport.CDN.Domain == "" {
			return fmt.Errorf("transport.cdn.domain is required when CDN is enabled")
		}
		switch c.Transport.CDN.Mode {
		case "", "h2", "grpc":
		default:
			return fmt.Errorf("invalid transport.cdn.mode: %q (must be \"h2\", \"grpc\", or empty)", c.Transport.CDN.Mode)
		}
	}

	// Transport.Reality validation
	if c.Transport.Reality.Enabled {
		if c.Transport.Reality.PublicKey == "" {
			return fmt.Errorf("transport.reality.public_key is required when Reality is enabled")
		}
	}

	// Transport.WebRTC validation
	if c.Transport.WebRTC.Enabled {
		if c.Transport.WebRTC.SignalURL == "" {
			return fmt.Errorf("transport.webrtc.signal_url is required when WebRTC is enabled")
		}
		if err := validateURL(c.Transport.WebRTC.SignalURL, "transport.webrtc.signal_url"); err != nil {
			return err
		}
	}

	// Log.Level validation
	switch c.Log.Level {
	case "debug", "info", "warn", "error", "":
	default:
		return fmt.Errorf("invalid log.level: %q", c.Log.Level)
	}

	// Log.Format validation
	switch c.Log.Format {
	case "text", "json", "":
	default:
		return fmt.Errorf("invalid log.format: %q", c.Log.Format)
	}

	// Inbounds validation
	seenInboundTags := make(map[string]bool)
	for i, ib := range c.Inbounds {
		if ib.Tag == "" {
			return fmt.Errorf("inbounds[%d].tag must not be empty", i)
		}
		if ib.Type == "" {
			return fmt.Errorf("inbounds[%d].type must not be empty", i)
		}
		if seenInboundTags[ib.Tag] {
			return fmt.Errorf("inbounds[%d].tag %q is duplicate", i, ib.Tag)
		}
		seenInboundTags[ib.Tag] = true
	}

	// Outbounds validation
	reservedOutboundTags := map[string]bool{"direct": true, "reject": true, "proxy": true}
	seenOutboundTags := make(map[string]bool)
	for i, ob := range c.Outbounds {
		if ob.Tag == "" {
			return fmt.Errorf("outbounds[%d].tag must not be empty", i)
		}
		if ob.Type == "" {
			return fmt.Errorf("outbounds[%d].type must not be empty", i)
		}
		if reservedOutboundTags[ob.Tag] {
			return fmt.Errorf("outbounds[%d].tag %q collides with built-in tag", i, ob.Tag)
		}
		if seenOutboundTags[ob.Tag] {
			return fmt.Errorf("outbounds[%d].tag %q is duplicate", i, ob.Tag)
		}
		seenOutboundTags[ob.Tag] = true
	}

	return nil
}

// Validate checks the server config for obviously wrong values.
func (c *ServerConfig) Validate() error {
	if c.Listen != "" {
		if _, _, err := net.SplitHostPort(c.Listen); err != nil {
			return fmt.Errorf("invalid listen address: %w", err)
		}
	}
	switch c.Cover.Mode {
	case "static", "reverse", "default", "":
	default:
		return fmt.Errorf("invalid cover.mode: %q", c.Cover.Mode)
	}

	// Cover.Mode == "reverse" requires valid ReverseURL
	if c.Cover.Mode == "reverse" {
		if c.Cover.ReverseURL == "" {
			return fmt.Errorf("cover.reverse_url is required when cover.mode is \"reverse\"")
		}
		if err := validateURL(c.Cover.ReverseURL, "cover.reverse_url"); err != nil {
			return err
		}
		// Block private/loopback/link-local literal IPs in reverse_url to
		// prevent SSRF. We only check literal IPs here (no DNS lookup) because
		// config validation must not perform network I/O.
		if u, err := url.Parse(c.Cover.ReverseURL); err == nil {
			if host := u.Hostname(); host != "" {
				if ip := net.ParseIP(host); ip != nil {
					if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
						return fmt.Errorf("cover.reverse_url must not point to a private or localhost address")
					}
				}
			}
		}
	}

	// Cover.Mode == "static" requires StaticDir
	if c.Cover.Mode == "static" {
		if c.Cover.StaticDir == "" {
			return fmt.Errorf("cover.static_dir is required when cover.mode is \"static\"")
		}
	}

	// Mesh.CIDR validation when mesh is enabled
	if c.Mesh.Enabled {
		if err := validateCIDR(c.Mesh.CIDR, "mesh.cidr"); err != nil {
			return err
		}
	}

	// Admin.Listen validation when admin is enabled
	if c.Admin.Enabled {
		if c.Admin.Listen != "" {
			if err := validateHostPort(c.Admin.Listen, "admin.listen"); err != nil {
				return err
			}
		}
	}

	// Debug.PprofListen validation
	if c.Debug.PprofListen != "" {
		if err := validateHostPort(c.Debug.PprofListen, "debug.pprof_listen"); err != nil {
			return err
		}
	}

	if c.Cluster.Enabled {
		if c.Cluster.NodeName == "" {
			return fmt.Errorf("cluster.node_name is required when cluster is enabled")
		}
		if c.Cluster.Secret == "" {
			return fmt.Errorf("cluster.secret is required when cluster is enabled")
		}
		for _, p := range c.Cluster.Peers {
			if p.Name == "" {
				return fmt.Errorf("cluster peer name is required")
			}
			if _, _, err := net.SplitHostPort(p.Addr); err != nil {
				return fmt.Errorf("invalid cluster peer address %q: %w", p.Addr, err)
			}
		}
		if c.Cluster.Interval != "" {
			if err := validateBoundedDuration(c.Cluster.Interval, "cluster.interval", 1*time.Second, 10*time.Minute); err != nil {
				return err
			}
		}
	}

	// DrainTimeout validation
	if c.DrainTimeout != "" {
		if err := validateBoundedDuration(c.DrainTimeout, "drain_timeout", 100*time.Millisecond, 60*time.Second); err != nil {
			return err
		}
	}
	return nil
}

// validateRuleChainEntry validates a single rule chain entry.
func validateRuleChainEntry(idx int, entry *RuleChainEntry) error {
	prefix := fmt.Sprintf("routing.rule_chain[%d]", idx)

	// Action is required. Built-in actions are "proxy", "direct", "reject".
	// Any other non-empty string is treated as a custom outbound tag and
	// will be resolved at runtime; unknown tags fail at connection time.
	if entry.Action == "" {
		return fmt.Errorf("%s: action is required", prefix)
	}

	// Logic must be "and", "or", or empty.
	switch strings.ToLower(entry.Logic) {
	case "", "and", "or":
	default:
		return fmt.Errorf("%s: invalid logic %q (must be \"and\" or \"or\")", prefix, entry.Logic)
	}

	// At least one match condition must be present.
	m := entry.Match
	hasCondition := len(m.Domain) > 0 || len(m.DomainSuffix) > 0 || len(m.DomainKeyword) > 0 ||
		len(m.GeoSite) > 0 || len(m.IPCIDR) > 0 || len(m.GeoIP) > 0 ||
		len(m.Process) > 0 || len(m.Protocol) > 0 || len(m.NetworkType) > 0
	if !hasCondition {
		return fmt.Errorf("%s: at least one match condition is required", prefix)
	}

	// Validate CIDRs parse correctly.
	for _, cidr := range m.IPCIDR {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("%s: invalid ip_cidr %q: %w", prefix, cidr, err)
		}
	}

	return nil
}
