package config

import (
	"net"
	"os"
	"path/filepath"

	"github.com/ggshr9/shuttle/internal/paths"
)

// DefaultListenPort is the default listen address for the server when none is specified.
const DefaultListenPort = ":443"

const (
	defaultGeoBase     = "https://raw.githubusercontent.com/Loyalsoldier/v2ray-rules-dat/release/"
	defaultDirectList  = defaultGeoBase + "direct-list.txt"
	defaultProxyList   = defaultGeoBase + "proxy-list.txt"
	defaultRejectList  = defaultGeoBase + "reject-list.txt"
	defaultGFWList     = defaultGeoBase + "gfw.txt"
	defaultCNCidr      = "https://raw.githubusercontent.com/misakaio/chnroutes2/master/chnroutes.txt"
	defaultPrivateCidr = "" // not needed, private ranges handled by router directly
)

// DefaultClientConfig returns a config with sensible defaults, ready for the GUI
// to display and let the user fill in server details.
func DefaultClientConfig() *ClientConfig {
	cfg := &ClientConfig{}
	applyClientDefaults(cfg)
	cfg.Proxy.SOCKS5.Enabled = true
	cfg.Proxy.HTTP.Enabled = true
	cfg.Transport.H3.Enabled = true
	cfg.Transport.Reality.Enabled = true
	cfg.Transport.CDN.Enabled = false // requires explicit domain configuration
	cfg.Congestion.Mode = "adaptive"
	cfg.Routing.GeoData.Enabled = true
	cfg.Routing.GeoData.AutoUpdate = true
	return cfg
}

func applyClientDefaults(cfg *ClientConfig) {
	if cfg.Transport.Preferred == "" {
		cfg.Transport.Preferred = "auto"
	}
	if cfg.Transport.PoolMaxIdle == 0 {
		cfg.Transport.PoolMaxIdle = 4
	}
	if cfg.Transport.PoolIdleTTL == "" {
		cfg.Transport.PoolIdleTTL = "60s"
	}
	if cfg.Transport.KeepaliveInterval == "" {
		cfg.Transport.KeepaliveInterval = "15s"
	}
	if cfg.Transport.KeepaliveTimeout == "" {
		cfg.Transport.KeepaliveTimeout = "5s"
	}
	// Determine bind address based on AllowLAN setting
	bindHost := "127.0.0.1"
	if cfg.Proxy.AllowLAN {
		bindHost = "0.0.0.0"
	}
	if cfg.Proxy.SOCKS5.Listen == "" {
		cfg.Proxy.SOCKS5.Listen = bindHost + ":1080"
	}
	if cfg.Proxy.HTTP.Listen == "" {
		cfg.Proxy.HTTP.Listen = bindHost + ":8080"
	}
	if cfg.Routing.Default == "" {
		cfg.Routing.Default = "proxy"
	}
	if cfg.Routing.DNS.Domestic == "" {
		cfg.Routing.DNS.Domestic = "223.5.5.5"
	}
	if cfg.Routing.DNS.Remote.Server == "" {
		cfg.Routing.DNS.Remote.Server = "https://1.1.1.1/dns-query"
	}
	if cfg.Routing.DNS.Remote.Via == "" {
		cfg.Routing.DNS.Remote.Via = "proxy"
	}
	applyGeoDataDefaults(&cfg.Routing.GeoData)
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Congestion.Mode == "" {
		cfg.Congestion.Mode = "adaptive"
	}
	// Auto-fill SNI from server address hostname
	if cfg.Server.SNI == "" && cfg.Server.Addr != "" {
		host := cfg.Server.Addr
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		// Only set SNI if it looks like a hostname (not an IP)
		if net.ParseIP(host) == nil {
			cfg.Server.SNI = host
		}
	}
	// Obfuscation defaults
	if cfg.Obfs.MaxDelay == "" {
		cfg.Obfs.MaxDelay = "50ms"
	}
	// P2P defaults
	if len(cfg.Mesh.P2P.STUNServers) == 0 {
		cfg.Mesh.P2P.STUNServers = []string{
			"stun.l.google.com:19302",
			"stun1.l.google.com:19302",
			"stun.cloudflare.com:3478",
		}
	}
	if cfg.Mesh.P2P.HolePunchTimeout == "" {
		cfg.Mesh.P2P.HolePunchTimeout = "10s"
	}
	if cfg.Mesh.P2P.DirectRetryInterval == "" {
		cfg.Mesh.P2P.DirectRetryInterval = "60s"
	}
	if cfg.Mesh.P2P.KeepAliveInterval == "" {
		cfg.Mesh.P2P.KeepAliveInterval = "30s"
	}
	if cfg.Mesh.P2P.FallbackThreshold == 0 {
		cfg.Mesh.P2P.FallbackThreshold = 0.3
	}
	// Yamux defaults
	if cfg.Yamux.MaxStreamWindowSize == 0 {
		cfg.Yamux.MaxStreamWindowSize = 256 * 1024 // 256KB
	}
	if cfg.Yamux.KeepAliveInterval == 0 {
		cfg.Yamux.KeepAliveInterval = 30 // seconds
	}
	if cfg.Yamux.ConnectionWriteTimeout == 0 {
		cfg.Yamux.ConnectionWriteTimeout = 10 // seconds
	}
	// Retry defaults
	if cfg.Retry.MaxAttempts == 0 {
		cfg.Retry.MaxAttempts = 3
	}
	if cfg.Retry.InitialBackoff == "" {
		cfg.Retry.InitialBackoff = "1s"
	}
	if cfg.Retry.MaxBackoff == "" {
		cfg.Retry.MaxBackoff = "30s"
	}
}

func applyGeoDataDefaults(g *GeoDataConfig) {
	if g.DataDir == "" {
		// Cache lives under the OS-appropriate user data dir so Windows
		// hits %AppData%\Shuttle\geodata and macOS hits
		// ~/Library/Application Support/Shuttle/geodata instead of
		// landing in ~/.shuttle/geodata everywhere. Geodata files are
		// regenerable from URLs, so users upgrading from the legacy
		// location just see a one-time re-download.
		scope := paths.ScopeUser
		if os.Getuid() == 0 {
			scope = paths.ScopeSystem
		}
		if dir := paths.Resolve(scope).ConfigDir; dir != "" {
			g.DataDir = filepath.Join(dir, "geodata")
		} else if home, err := os.UserHomeDir(); err == nil {
			g.DataDir = filepath.Join(home, ".shuttle", "geodata")
		}
	}
	if g.UpdateInterval == "" {
		g.UpdateInterval = "24h"
	}
	if g.DirectListURL == "" {
		g.DirectListURL = defaultDirectList
	}
	if g.ProxyListURL == "" {
		g.ProxyListURL = defaultProxyList
	}
	if g.RejectListURL == "" {
		g.RejectListURL = defaultRejectList
	}
	if g.GFWListURL == "" {
		g.GFWListURL = defaultGFWList
	}
	if g.CNCidrURL == "" {
		g.CNCidrURL = defaultCNCidr
	}
	if g.PrivateCidrURL == "" {
		g.PrivateCidrURL = defaultPrivateCidr
	}
}

// DefaultServerConfig returns a config with sensible defaults.
func DefaultServerConfig() *ServerConfig {
	cfg := &ServerConfig{}
	applyServerDefaults(cfg)
	cfg.Transport.H3.Enabled = true
	cfg.Transport.H3.PathPrefix = "/h3"
	cfg.Transport.Reality.Enabled = true
	cfg.Transport.Reality.TargetSNI = "www.microsoft.com"
	cfg.Transport.Reality.TargetAddr = "www.microsoft.com:443"
	return cfg
}

func applyServerDefaults(cfg *ServerConfig) {
	if cfg.Listen == "" {
		cfg.Listen = DefaultListenPort
	}
	if cfg.Cover.Mode == "" {
		cfg.Cover.Mode = "default"
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Mesh.CIDR == "" {
		cfg.Mesh.CIDR = "10.7.0.0/24"
	}
	if cfg.Admin.Listen == "" {
		cfg.Admin.Listen = "127.0.0.1:9090"
	}
	if cfg.MaxStreams == 0 {
		cfg.MaxStreams = 1024
	}
}
