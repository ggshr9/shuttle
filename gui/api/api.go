package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ggshr9/shuttle/config"
	"github.com/ggshr9/shuttle/connlog"
	"github.com/ggshr9/shuttle/engine"
	"github.com/ggshr9/shuttle/internal/healthcheck"
	"github.com/ggshr9/shuttle/server"
	"github.com/ggshr9/shuttle/speedtest"
	"github.com/ggshr9/shuttle/stats"
	"github.com/ggshr9/shuttle/subscription"
	"github.com/ggshr9/shuttle/sysproxy"
	"github.com/ggshr9/shuttle/update"
)

// apiStartTime records when the API package was initialised, used for uptime calculation.
var apiStartTime = time.Now()

// HandlerConfig holds all dependencies for constructing the API handler.
type HandlerConfig struct {
	Engine       *engine.Engine
	SubMgr       *subscription.Manager
	Stats        *stats.Storage
	ConnLog      *connlog.Storage
	SpeedHistory *speedtest.HistoryStorage
	Events       *EventQueue // optional; nil disables /api/events + /ws/events
	AuthToken    string      // empty = no auth
	// Heartbeat backs /api/health/live. The lifecycle owner (typically
	// gui/api.Server) is responsible for constructing and ticking it.
	// When nil, /api/health/live always returns 200 — the conservative
	// "no liveness signal tracked" semantics for direct callers that
	// don't run a heartbeat goroutine.
	Heartbeat *healthcheck.Heartbeat
}

// NewHandler creates the HTTP handler for the shuttle API.
// All fields except Engine are optional. If SubMgr is nil, a new empty
// subscription.Manager is created. If AuthToken is non-empty, all /api/
// endpoints require Bearer token authentication.
func NewHandler(cfg HandlerConfig) http.Handler {
	if cfg.SubMgr == nil {
		mgr := subscription.NewManager()
		// Inherit AllowPrivateNetworks from engine config (for sandbox testing)
		if cfg.Engine != nil {
			c := cfg.Engine.Config()
			mgr.SetAllowPrivateNetworks(c.AllowPrivateNetworks)
		}
		cfg.SubMgr = mgr
	}

	mux := http.NewServeMux()
	updateChecker := update.NewChecker()

	registerHealthzRoute(mux)
	registerDeepHealthRoutes(mux, cfg.Engine, cfg.Heartbeat)
	registerStatusRoutes(mux, cfg.Engine)
	registerConfigRoutes(mux, cfg.Engine)
	registerProxyRoutes(mux, cfg.Engine)
	registerRoutingRoutes(mux, cfg.Engine)
	registerSubscriptionRoutes(mux, cfg.Engine, cfg.SubMgr)
	registerStatsRoutes(mux, cfg.Engine, cfg.Stats, cfg.ConnLog)
	registerSpeedtestRoutes(mux, cfg.Engine, cfg.SpeedHistory)
	registerMiscRoutes(mux, cfg.Engine, cfg.SubMgr, updateChecker)
	registerPrometheusRoutes(mux, cfg.Engine)
	registerTransportRoutes(mux, cfg.Engine)
	registerMeshRoutes(mux, cfg.Engine)
	registerGroupRoutes(mux, cfg.Engine)
	registerProviderRoutes(mux, cfg.Engine)
	registerMigrateRoutes(mux)
	registerEventsRoutes(mux, cfg.Events)

	var handler http.Handler = corsMiddleware(mux)
	if cfg.AuthToken != "" {
		handler = authMiddleware(cfg.AuthToken, handler)
	}
	return handler
}

// ---------------------------------------------------------------------------
// GenerateAuthToken generates a cryptographically random auth token.
// ---------------------------------------------------------------------------

func GenerateAuthToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate auth token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

// authMiddleware returns HTTP middleware that enforces Bearer token authentication
// on all API endpoints. Static file requests (paths not starting with /api/) are exempt.
func authMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Exempt non-API paths (static files, etc.)
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		// Check Authorization header using constant-time comparison
		// to prevent timing attacks on the auth token.
		auth := r.Header.Get("Authorization")
		const prefix = "Bearer "
		headerOK := strings.HasPrefix(auth, prefix) &&
			subtle.ConstantTimeCompare([]byte(auth[len(prefix):]), []byte(token)) == 1
		if !headerOK {
			// Also check query param for WebSocket connections
			qToken := r.URL.Query().Get("token")
			if subtle.ConstantTimeCompare([]byte(qToken), []byte(token)) != 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// corsMiddleware adds CORS headers for dev mode (Vite on different port).
// Only allows localhost origins for security.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		// Only allow localhost origins for security
		if strings.HasPrefix(origin, "http://localhost:") ||
			strings.HasPrefix(origin, "http://127.0.0.1:") ||
			origin == "http://localhost" ||
			origin == "http://127.0.0.1" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---------------------------------------------------------------------------
// Helpers — shared by route files
// ---------------------------------------------------------------------------

const maxRequestBody = 10 * 1024 * 1024 // 10MB

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, maxRequestBody)).Decode(v)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Log but don't try to write error (headers already sent)
		slog.Debug("writeJSON encode error", "err", err)
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// setSystemProxy configures the system proxy based on the current config.
func setSystemProxy(cfg *config.ClientConfig) {
	proxyCfg := sysproxy.ProxyConfig{
		Enable: true,
		Bypass: sysproxy.DefaultBypass(),
	}

	// Extract host:port from listen addresses
	if cfg.Proxy.HTTP.Enabled && cfg.Proxy.HTTP.Listen != "" {
		proxyCfg.HTTPAddr = normalizeListenAddr(cfg.Proxy.HTTP.Listen)
	}
	if cfg.Proxy.SOCKS5.Enabled && cfg.Proxy.SOCKS5.Listen != "" {
		proxyCfg.SOCKSAddr = normalizeListenAddr(cfg.Proxy.SOCKS5.Listen)
	}

	_ = sysproxy.Set(proxyCfg)
}

// normalizeListenAddr converts listen addresses like ":1080" or "0.0.0.0:1080" to "127.0.0.1:1080"
func normalizeListenAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		// Handle non-bracketed IPv6 like ":::1080" by trying with brackets.
		if strings.Count(addr, ":") >= 2 {
			lastColon := strings.LastIndex(addr, ":")
			host, port, err = net.SplitHostPort("[" + addr[:lastColon] + "]" + addr[lastColon:])
		}
		if err != nil {
			return addr
		}
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return host + ":" + port
}

// validateProbeURL checks that a probe URL is safe to request.
// Rejects non-HTTP(S) schemes and (unless allowPrivate is true) localhost /
// private-IP targets to prevent SSRF.
//
// allowPrivate=true skips the address-class block — used in sandbox/testing
// contexts where the probe target is intentionally on a private network.
// Mirrors the AllowPrivateNetworks behavior in server.HandlerConfig and
// subscription.Manager.
func validateProbeURL(rawURL string, allowPrivate bool) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("unsupported scheme %q: only http and https are allowed", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL must have a hostname")
	}
	if allowPrivate {
		return nil
	}
	// Block localhost name before IP check to avoid DNS lookup.
	if host == "localhost" {
		return fmt.Errorf("probing localhost is not allowed")
	}
	// Block private, loopback, link-local, and cloud-metadata ranges.
	// For literal IPs, check directly. For hostnames, use IsBlockedTarget
	// which resolves DNS before checking (prevents DNS rebinding).
	if ip := net.ParseIP(host); ip != nil {
		if server.IsBlockedIP(ip) {
			return fmt.Errorf("probing private/localhost/link-local addresses is not allowed")
		}
	} else {
		if server.IsBlockedTarget(host) {
			return fmt.Errorf("probing private/localhost/link-local addresses is not allowed (hostname %q resolves to a blocked address)", host)
		}
	}
	return nil
}

// getLANAddresses returns all non-loopback IPv4 addresses on the device.
// Useful for displaying to users when AllowLAN is enabled.
func getLANAddresses() []string {
	var addrs []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return addrs
	}
	for _, iface := range ifaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		ifAddrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range ifAddrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			// Only include IPv4 addresses, skip loopback
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}
			addrs = append(addrs, ip.String())
		}
	}
	return addrs
}

const redacted = "***"

// redactClientConfig replaces sensitive fields (passwords, tokens, private keys)
// with a placeholder so they are not leaked via the API.
func redactClientConfig(cfg *config.ClientConfig) {
	// Active server
	if cfg.Server.Password != "" {
		cfg.Server.Password = redacted
	}
	// Saved server list
	for i := range cfg.Servers {
		if cfg.Servers[i].Password != "" {
			cfg.Servers[i].Password = redacted
		}
	}
}
