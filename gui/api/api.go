package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/shuttle-proxy/shuttle/config"
	"github.com/shuttle-proxy/shuttle/engine"
	"github.com/shuttle-proxy/shuttle/internal/procnet"
	"github.com/shuttle-proxy/shuttle/speedtest"
	"github.com/shuttle-proxy/shuttle/stats"
	"github.com/shuttle-proxy/shuttle/subscription"
	"github.com/shuttle-proxy/shuttle/sysproxy"
	"github.com/shuttle-proxy/shuttle/update"
)

// Handler creates the HTTP handler for the shuttle API.
func Handler(eng *engine.Engine) http.Handler {
	return HandlerWithOptions(eng, subscription.NewManager(), nil)
}

// HandlerWithSubscriptions creates the HTTP handler with subscription support.
func HandlerWithSubscriptions(eng *engine.Engine, subMgr *subscription.Manager) http.Handler {
	return HandlerWithOptions(eng, subMgr, nil)
}

// HandlerWithOptions creates the HTTP handler with all options.
func HandlerWithOptions(eng *engine.Engine, subMgr *subscription.Manager, statsStore *stats.Storage) http.Handler {
	mux := http.NewServeMux()
	updateChecker := update.NewChecker()

	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, eng.Status())
	})

	mux.HandleFunc("POST /api/connect", func(w http.ResponseWriter, r *http.Request) {
		if err := eng.Start(r.Context()); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}

		// Set system proxy if enabled
		cfg := eng.Config()
		if cfg.Proxy.SystemProxy.Enabled {
			setSystemProxy(&cfg)
		}

		writeJSON(w, map[string]string{"status": "connected"})
	})

	mux.HandleFunc("POST /api/disconnect", func(w http.ResponseWriter, r *http.Request) {
		// Clear system proxy first
		cfg := eng.Config()
		if cfg.Proxy.SystemProxy.Enabled {
			sysproxy.Clear()
		}

		if err := eng.Stop(); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, map[string]string{"status": "disconnected"})
	})

	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		cfg := eng.Config()
		writeJSON(w, &cfg)
	})

	mux.HandleFunc("PUT /api/config", func(w http.ResponseWriter, r *http.Request) {
		var cfg config.ClientConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()
		if err := eng.Reload(&cfg); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]string{"status": "reloaded"})
	})

	mux.HandleFunc("GET /api/config/servers", func(w http.ResponseWriter, r *http.Request) {
		cfg := eng.Config()
		writeJSON(w, map[string]any{
			"active":  cfg.Server,
			"servers": cfg.Servers,
		})
	})

	mux.HandleFunc("PUT /api/config/servers", func(w http.ResponseWriter, r *http.Request) {
		var srv config.ServerEndpoint
		if err := json.NewDecoder(r.Body).Decode(&srv); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()
		if srv.Addr == "" {
			writeError(w, http.StatusBadRequest, "addr is required")
			return
		}
		cfg := eng.Config()
		cfg.Server = srv
		if err := eng.Reload(&cfg); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]string{"status": "updated"})
	})

	mux.HandleFunc("POST /api/config/servers", func(w http.ResponseWriter, r *http.Request) {
		var srv config.ServerEndpoint
		if err := json.NewDecoder(r.Body).Decode(&srv); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()
		if srv.Addr == "" {
			writeError(w, http.StatusBadRequest, "addr is required")
			return
		}
		cfg := eng.Config()
		cfg.Servers = append(cfg.Servers, srv)
		eng.SetConfig(&cfg)
		writeJSON(w, map[string]string{"status": "added"})
	})

	mux.HandleFunc("DELETE /api/config/servers", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Addr string `json:"addr"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()
		if req.Addr == "" {
			writeError(w, http.StatusBadRequest, "addr is required")
			return
		}
		cfg := eng.Config()
		filtered := make([]config.ServerEndpoint, 0, len(cfg.Servers))
		for _, s := range cfg.Servers {
			if s.Addr != req.Addr {
				filtered = append(filtered, s)
			}
		}
		cfg.Servers = filtered
		eng.SetConfig(&cfg)
		writeJSON(w, map[string]string{"status": "deleted"})
	})

	// Import configuration (base64, JSON, or shuttle:// URI)
	mux.HandleFunc("POST /api/config/import", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Data string `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()

		result, err := config.ImportConfig(req.Data)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		// Add imported servers to config
		cfg := eng.Config()
		existingAddrs := make(map[string]bool)
		for _, s := range cfg.Servers {
			existingAddrs[s.Addr] = true
		}

		added := 0
		for _, srv := range result.Servers {
			if !existingAddrs[srv.Addr] {
				cfg.Servers = append(cfg.Servers, srv)
				existingAddrs[srv.Addr] = true
				added++
			}
		}
		eng.SetConfig(&cfg)

		writeJSON(w, map[string]any{
			"status":  "imported",
			"added":   added,
			"total":   len(result.Servers),
			"servers": result.Servers,
			"errors":  result.Errors,
		})
	})

	// Export configuration
	mux.HandleFunc("GET /api/config/export", func(w http.ResponseWriter, r *http.Request) {
		format := r.URL.Query().Get("format")
		if format == "" {
			format = "json"
		}

		cfg := eng.Config()

		switch format {
		case "json":
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Disposition", "attachment; filename=shuttle-config.json")
			json.NewEncoder(w).Encode(&cfg)
		case "yaml":
			data, err := config.ExportConfig(&cfg, "yaml")
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			w.Header().Set("Content-Type", "text/yaml")
			w.Header().Set("Content-Disposition", "attachment; filename=shuttle-config.yaml")
			w.Write(data)
		case "uri":
			data, err := config.ExportConfig(&cfg, "uri")
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Disposition", "attachment; filename=shuttle-servers.txt")
			w.Write(data)
		default:
			writeError(w, http.StatusBadRequest, "unsupported format: "+format)
		}
	})

	// Full backup - exports complete configuration including subscriptions
	mux.HandleFunc("GET /api/backup", func(w http.ResponseWriter, r *http.Request) {
		cfg := eng.Config()

		// Get subscriptions from manager
		subs := []*subscription.Subscription{}
		if subMgr != nil {
			subs = subMgr.List()
		}

		backup := map[string]interface{}{
			"version":       1,
			"config":        cfg,
			"subscriptions": subs,
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=shuttle-backup.json")
		json.NewEncoder(w).Encode(backup)
	})

	// Restore from backup
	mux.HandleFunc("POST /api/restore", func(w http.ResponseWriter, r *http.Request) {
		var backup struct {
			Version       int                         `json:"version"`
			Config        config.ClientConfig         `json:"config"`
			Subscriptions []*subscription.Subscription `json:"subscriptions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&backup); err != nil {
			writeError(w, http.StatusBadRequest, "invalid backup format: "+err.Error())
			return
		}
		r.Body.Close()

		// Restore subscriptions
		if subMgr != nil && len(backup.Subscriptions) > 0 {
			for _, sub := range backup.Subscriptions {
				subMgr.Add(sub.Name, sub.URL)
			}
		}

		// Restore configuration
		if err := eng.Reload(&backup.Config); err != nil {
			writeError(w, http.StatusInternalServerError, "restore failed: "+err.Error())
			return
		}

		writeJSON(w, map[string]interface{}{
			"status":       "restored",
			"servers":      len(backup.Config.Servers),
			"subscriptions": len(backup.Subscriptions),
		})
	})

	mux.HandleFunc("GET /api/routing/rules", func(w http.ResponseWriter, r *http.Request) {
		cfg := eng.Config()
		writeJSON(w, cfg.Routing)
	})

	mux.HandleFunc("PUT /api/routing/rules", func(w http.ResponseWriter, r *http.Request) {
		var routing config.RoutingConfig
		if err := json.NewDecoder(r.Body).Decode(&routing); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()
		cfg := eng.Config()
		cfg.Routing = routing
		if err := eng.Reload(&cfg); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]string{"status": "updated"})
	})

	// Export routing rules
	mux.HandleFunc("GET /api/routing/export", func(w http.ResponseWriter, r *http.Request) {
		cfg := eng.Config()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=shuttle-rules.json")
		json.NewEncoder(w).Encode(cfg.Routing)
	})

	// Import routing rules
	mux.HandleFunc("POST /api/routing/import", func(w http.ResponseWriter, r *http.Request) {
		var routing config.RoutingConfig
		if err := json.NewDecoder(r.Body).Decode(&routing); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()

		cfg := eng.Config()
		// Merge imported rules with existing
		existingCount := len(cfg.Routing.Rules)
		cfg.Routing.Rules = append(cfg.Routing.Rules, routing.Rules...)

		// Apply default action if specified in import
		if routing.Default != "" {
			cfg.Routing.Default = routing.Default
		}

		if err := eng.Reload(&cfg); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, map[string]any{
			"status":   "imported",
			"added":    len(routing.Rules),
			"total":    len(cfg.Routing.Rules),
			"existing": existingCount,
		})
	})

	// Routing templates
	mux.HandleFunc("GET /api/routing/templates", func(w http.ResponseWriter, r *http.Request) {
		templates := []map[string]any{
			{
				"id":          "bypass-cn",
				"name":        "Bypass China",
				"description": "Direct connection for China sites, proxy for others",
				"rules": []config.RouteRule{
					{GeoSite: "cn", Action: "direct"},
					{GeoIP: "CN", Action: "direct"},
					{GeoSite: "private", Action: "direct"},
				},
				"default": "proxy",
			},
			{
				"id":          "proxy-all",
				"name":        "Proxy All",
				"description": "Route all traffic through proxy",
				"rules": []config.RouteRule{
					{GeoSite: "private", Action: "direct"},
				},
				"default": "proxy",
			},
			{
				"id":          "direct-all",
				"name":        "Direct All",
				"description": "Direct connection for all traffic",
				"rules":       []config.RouteRule{},
				"default":     "direct",
			},
			{
				"id":          "block-ads",
				"name":        "Block Ads",
				"description": "Block advertising and tracking domains",
				"rules": []config.RouteRule{
					{GeoSite: "category-ads-all", Action: "reject"},
				},
				"default": "proxy",
			},
		}
		writeJSON(w, templates)
	})

	// Apply template
	mux.HandleFunc("POST /api/routing/templates/", func(w http.ResponseWriter, r *http.Request) {
		templateID := strings.TrimPrefix(r.URL.Path, "/api/routing/templates/")
		if templateID == "" {
			writeError(w, http.StatusBadRequest, "template id required")
			return
		}

		var template config.RoutingConfig
		switch templateID {
		case "bypass-cn":
			template = config.RoutingConfig{
				Default: "proxy",
				Rules: []config.RouteRule{
					{GeoSite: "cn", Action: "direct"},
					{GeoIP: "CN", Action: "direct"},
					{GeoSite: "private", Action: "direct"},
				},
			}
		case "proxy-all":
			template = config.RoutingConfig{
				Default: "proxy",
				Rules: []config.RouteRule{
					{GeoSite: "private", Action: "direct"},
				},
			}
		case "direct-all":
			template = config.RoutingConfig{
				Default: "direct",
				Rules:   []config.RouteRule{},
			}
		case "block-ads":
			template = config.RoutingConfig{
				Default: "proxy",
				Rules: []config.RouteRule{
					{GeoSite: "category-ads-all", Action: "reject"},
				},
			}
		default:
			writeError(w, http.StatusNotFound, "template not found: "+templateID)
			return
		}

		cfg := eng.Config()
		// Preserve DNS settings when applying template
		template.DNS = cfg.Routing.DNS
		cfg.Routing = template

		if err := eng.Reload(&cfg); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, map[string]string{"status": "applied", "template": templateID})
	})

	mux.HandleFunc("GET /api/processes", func(w http.ResponseWriter, r *http.Request) {
		procs, err := procnet.ListNetworkProcesses()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, procs)
	})

	// Subscription endpoints
	mux.HandleFunc("GET /api/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, subMgr.List())
	})

	mux.HandleFunc("POST /api/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()

		if req.URL == "" {
			writeError(w, http.StatusBadRequest, "url is required")
			return
		}

		sub, err := subMgr.Add(req.Name, req.URL)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Auto-refresh after adding
		subMgr.Refresh(r.Context(), sub.ID)
		sub, _ = subMgr.Get(sub.ID)

		// Save to config
		cfg := eng.Config()
		cfg.Subscriptions = subMgr.ToConfig()
		eng.SetConfig(&cfg)

		writeJSON(w, sub)
	})

	mux.HandleFunc("PUT /api/subscriptions/", func(w http.ResponseWriter, r *http.Request) {
		// Extract ID from path: /api/subscriptions/{id}/refresh
		path := strings.TrimPrefix(r.URL.Path, "/api/subscriptions/")
		parts := strings.Split(path, "/")
		if len(parts) < 2 || parts[1] != "refresh" {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		id := parts[0]

		sub, err := subMgr.Refresh(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, sub)
	})

	mux.HandleFunc("DELETE /api/subscriptions/", func(w http.ResponseWriter, r *http.Request) {
		// Extract ID from path: /api/subscriptions/{id}
		id := strings.TrimPrefix(r.URL.Path, "/api/subscriptions/")
		if id == "" {
			writeError(w, http.StatusBadRequest, "id is required")
			return
		}

		if err := subMgr.Remove(id); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		// Save to config
		cfg := eng.Config()
		cfg.Subscriptions = subMgr.ToConfig()
		eng.SetConfig(&cfg)

		writeJSON(w, map[string]string{"status": "deleted"})
	})

	// Speedtest endpoint
	mux.HandleFunc("POST /api/speedtest", func(w http.ResponseWriter, r *http.Request) {
		cfg := eng.Config()

		// Collect all servers to test
		var servers []speedtest.Server
		if cfg.Server.Addr != "" {
			servers = append(servers, speedtest.Server{
				Addr:     cfg.Server.Addr,
				Name:     cfg.Server.Name,
				Password: cfg.Server.Password,
				SNI:      cfg.Server.SNI,
			})
		}
		for _, srv := range cfg.Servers {
			servers = append(servers, speedtest.Server{
				Addr:     srv.Addr,
				Name:     srv.Name,
				Password: srv.Password,
				SNI:      srv.SNI,
			})
		}

		if len(servers) == 0 {
			writeError(w, http.StatusBadRequest, "no servers configured")
			return
		}

		tester := speedtest.NewTester(nil)
		results := tester.TestAll(r.Context(), servers)
		speedtest.SortByLatency(results)

		writeJSON(w, map[string]any{
			"results": results,
		})
	})

	// Auto-select best server based on latency
	mux.HandleFunc("POST /api/config/servers/auto-select", func(w http.ResponseWriter, r *http.Request) {
		cfg := eng.Config()

		// Collect all servers to test
		var servers []speedtest.Server
		serverMap := make(map[string]config.ServerEndpoint)

		for _, srv := range cfg.Servers {
			servers = append(servers, speedtest.Server{
				Addr:     srv.Addr,
				Name:     srv.Name,
				Password: srv.Password,
				SNI:      srv.SNI,
			})
			serverMap[srv.Addr] = srv
		}

		if len(servers) == 0 {
			writeError(w, http.StatusBadRequest, "no servers to select from")
			return
		}

		tester := speedtest.NewTester(nil)
		results := tester.TestAll(r.Context(), servers)
		speedtest.SortByLatency(results)

		// Find the best available server
		var best *speedtest.TestResult
		for i := range results {
			if results[i].Available {
				best = &results[i]
				break
			}
		}

		if best == nil {
			writeError(w, http.StatusServiceUnavailable, "no available servers")
			return
		}

		// Switch to the best server
		bestServer := serverMap[best.ServerAddr]
		cfg.Server = bestServer
		if err := eng.Reload(&cfg); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, map[string]any{
			"status":  "selected",
			"server":  bestServer,
			"latency": best.LatencyMs,
		})
	})

	// WebSocket speedtest stream
	mux.HandleFunc("GET /api/speedtest/stream", func(w http.ResponseWriter, r *http.Request) {
		handleSpeedtestWS(w, r, eng)
	})

	// Stats history endpoint
	mux.HandleFunc("GET /api/stats/history", func(w http.ResponseWriter, r *http.Request) {
		days := 7
		if d := r.URL.Query().Get("days"); d != "" {
			if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 && parsed <= 90 {
				days = parsed
			}
		}

		if statsStore != nil {
			writeJSON(w, map[string]any{
				"history": statsStore.GetHistory(days),
				"total":   statsStore.GetTotal(),
			})
		} else {
			// Return empty history if stats storage not configured
			writeJSON(w, map[string]any{
				"history": []stats.DailyStats{},
				"total":   stats.DailyStats{},
			})
		}
	})

	// Log export endpoint
	mux.HandleFunc("GET /api/logs/export", func(w http.ResponseWriter, r *http.Request) {
		// Get recent logs from the log buffer (if available)
		// For now, export current engine status and info
		cfg := eng.Config()
		status := eng.Status()

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=shuttle-logs.txt")

		w.Write([]byte("Shuttle Log Export\n"))
		w.Write([]byte("==================\n\n"))
		w.Write([]byte("Engine Status:\n"))
		w.Write([]byte("  State: " + status.State + "\n"))
		w.Write([]byte("  Transport: " + status.Transport + "\n"))
		w.Write([]byte("  Active Connections: " + fmt.Sprintf("%d", status.ActiveConns) + "\n"))
		w.Write([]byte("  Total Connections: " + fmt.Sprintf("%d", status.TotalConns) + "\n"))
		w.Write([]byte("  Bytes Sent: " + fmt.Sprintf("%d", status.BytesSent) + "\n"))
		w.Write([]byte("  Bytes Received: " + fmt.Sprintf("%d", status.BytesReceived) + "\n\n"))

		w.Write([]byte("Configuration:\n"))
		w.Write([]byte("  Server: " + cfg.Server.Addr + "\n"))
		w.Write([]byte("  Log Level: " + cfg.Log.Level + "\n"))
		w.Write([]byte("  SOCKS5: " + cfg.Proxy.SOCKS5.Listen + "\n"))
		w.Write([]byte("  HTTP: " + cfg.Proxy.HTTP.Listen + "\n"))
	})

	// WebSocket endpoints — use GET method filter
	mux.HandleFunc("GET /api/logs", func(w http.ResponseWriter, r *http.Request) {
		handleEventWS(w, r, eng, engine.EventLog)
	})

	mux.HandleFunc("GET /api/speed", func(w http.ResponseWriter, r *http.Request) {
		handleEventWS(w, r, eng, engine.EventSpeedTick)
	})

	mux.HandleFunc("GET /api/connections", func(w http.ResponseWriter, r *http.Request) {
		handleEventWS(w, r, eng, engine.EventConnection)
	})

	// Update check endpoint
	mux.HandleFunc("GET /api/update/check", func(w http.ResponseWriter, r *http.Request) {
		force := r.URL.Query().Get("force") == "true"
		info, err := updateChecker.Check(force)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, info)
	})

	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{
			"version": update.GetCurrentVersion(),
		})
	})

	return corsMiddleware(mux)
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
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
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

	sysproxy.Set(proxyCfg)
}

// normalizeListenAddr converts listen addresses like ":1080" or "0.0.0.0:1080" to "127.0.0.1:1080"
func normalizeListenAddr(addr string) string {
	host, port, err := splitHostPort(addr)
	if err != nil {
		return addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return host + ":" + port
}

func splitHostPort(addr string) (host, port string, err error) {
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		return "", "", fmt.Errorf("no port in address")
	}
	return addr[:idx], addr[idx+1:], nil
}
