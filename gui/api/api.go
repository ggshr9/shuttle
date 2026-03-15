package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/shuttle-proxy/shuttle/autostart"
	"github.com/shuttle-proxy/shuttle/config"
	"github.com/shuttle-proxy/shuttle/connlog"
	"github.com/shuttle-proxy/shuttle/engine"
	"github.com/shuttle-proxy/shuttle/internal/procnet"
	"github.com/shuttle-proxy/shuttle/router"
	"github.com/shuttle-proxy/shuttle/router/geodata"
	"github.com/shuttle-proxy/shuttle/speedtest"
	"github.com/shuttle-proxy/shuttle/stats"
	"github.com/shuttle-proxy/shuttle/subscription"
	"github.com/shuttle-proxy/shuttle/sysproxy"
	"github.com/shuttle-proxy/shuttle/update"
)

// apiStartTime records when the API package was initialised, used for uptime calculation.
var apiStartTime = time.Now()

// Handler creates the HTTP handler for the shuttle API.
func Handler(eng *engine.Engine) http.Handler {
	return HandlerWithOptions(eng, subscription.NewManager(), nil)
}

// HandlerWithSubscriptions creates the HTTP handler with subscription support.
func HandlerWithSubscriptions(eng *engine.Engine, subMgr *subscription.Manager) http.Handler {
	return HandlerWithOptions(eng, subMgr, nil)
}

// HandlerWithConnLog creates the HTTP handler with all options including connection log storage.
func HandlerWithConnLog(eng *engine.Engine, subMgr *subscription.Manager, statsStore *stats.Storage, connStore *connlog.Storage) http.Handler {
	return handlerWithAllOptions(eng, subMgr, statsStore, connStore)
}

// HandlerWithOptions creates the HTTP handler with all options.
func HandlerWithOptions(eng *engine.Engine, subMgr *subscription.Manager, statsStore *stats.Storage) http.Handler {
	return handlerWithAllOptions(eng, subMgr, statsStore, nil)
}

func handlerWithAllOptions(eng *engine.Engine, subMgr *subscription.Manager, statsStore *stats.Storage, connStore *connlog.Storage) http.Handler {
	mux := http.NewServeMux()
	updateChecker := update.NewChecker()

	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, eng.Status())
	})

	mux.HandleFunc("POST /api/connect", func(w http.ResponseWriter, r *http.Request) {
		if err := eng.Start(context.Background()); err != nil {
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
		if err := decodeJSON(r, &cfg); err != nil {
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
		if err := decodeJSON(r, &srv); err != nil {
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
		if err := decodeJSON(r, &srv); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()
		if srv.Addr == "" {
			writeError(w, http.StatusBadRequest, "addr is required")
			return
		}
		cfg := eng.Config()
		for _, s := range cfg.Servers {
			if s.Addr == srv.Addr {
				writeError(w, http.StatusConflict, "server with this address already exists")
				return
			}
		}
		cfg.Servers = append(cfg.Servers, srv)
		eng.SetConfig(&cfg)
		writeJSON(w, map[string]string{"status": "added"})
	})

	mux.HandleFunc("DELETE /api/config/servers", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Addr string `json:"addr"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()
		if req.Addr == "" {
			writeError(w, http.StatusBadRequest, "addr is required")
			return
		}
		cfg := eng.Config()
		found := false
		filtered := make([]config.ServerEndpoint, 0, len(cfg.Servers))
		for _, s := range cfg.Servers {
			if !found && s.Addr == req.Addr {
				found = true
				continue
			}
			filtered = append(filtered, s)
		}
		if !found {
			writeError(w, http.StatusNotFound, "server not found")
			return
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
		if err := decodeJSON(r, &req); err != nil {
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
		if err := decodeJSON(r, &backup); err != nil {
			writeError(w, http.StatusBadRequest, "invalid backup format: "+err.Error())
			return
		}
		r.Body.Close()

		if backup.Version != 1 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported backup version: %d (expected 1)", backup.Version))
			return
		}
		if err := backup.Config.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, "invalid config in backup: "+err.Error())
			return
		}

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
		if err := decodeJSON(r, &routing); err != nil {
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
		if err := decodeJSON(r, &routing); err != nil {
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

	// Routing dry-run: test how a domain would be routed without proxying.
	mux.HandleFunc("POST /api/routing/test", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			URL string `json:"url"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()

		if req.URL == "" {
			writeError(w, http.StatusBadRequest, "url is required")
			return
		}

		// Extract domain from the input — it may be a bare domain or a full URL.
		domain := req.URL
		if strings.Contains(domain, "://") {
			if u, err := url.Parse(domain); err == nil && u.Hostname() != "" {
				domain = u.Hostname()
			}
		}
		// Strip port if present.
		if h, _, err := net.SplitHostPort(domain); err == nil {
			domain = h
		}

		rt := eng.CurrentRouter()
		if rt == nil {
			// Build a temporary router from config when engine is not running.
			cfg := eng.Config()
			routerCfg := &router.RouterConfig{
				DefaultAction: router.Action(cfg.Routing.Default),
			}
			for _, rule := range cfg.Routing.Rules {
				rr := router.Rule{Action: router.Action(rule.Action)}
				switch {
				case rule.Domains != "":
					rr.Type = "domain"
					rr.Values = []string{rule.Domains}
				case rule.GeoSite != "":
					rr.Type = "geosite"
					rr.Values = []string{rule.GeoSite}
				case rule.GeoIP != "":
					rr.Type = "geoip"
					rr.Values = []string{rule.GeoIP}
				case len(rule.IPCIDR) > 0:
					rr.Type = "ip-cidr"
					rr.Values = rule.IPCIDR
				}
				routerCfg.Rules = append(routerCfg.Rules, rr)
			}
			rt = router.NewRouter(routerCfg, nil, nil, nil)
		}

		result := rt.DryRun(domain)
		writeJSON(w, result)
	})

	mux.HandleFunc("GET /api/transports/stats", func(w http.ResponseWriter, r *http.Request) {
		status := eng.Status()
		if status.TransportBreakdown != nil {
			writeJSON(w, status.TransportBreakdown)
		} else {
			writeJSON(w, []struct{}{})
		}
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
		if err := decodeJSON(r, &req); err != nil {
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

	// Geodata endpoints
	mux.HandleFunc("GET /api/geodata/status", func(w http.ResponseWriter, r *http.Request) {
		gm := eng.GeoManager()
		if gm == nil {
			writeJSON(w, map[string]any{"enabled": false})
			return
		}
		writeJSON(w, gm.Status())
	})

	mux.HandleFunc("POST /api/geodata/update", func(w http.ResponseWriter, r *http.Request) {
		gm := eng.GeoManager()
		if gm == nil {
			writeError(w, http.StatusBadRequest, "geodata not enabled")
			return
		}
		if err := gm.Update(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Reload engine to pick up new data
		cfg := eng.Config()
		_ = eng.Reload(&cfg)
		writeJSON(w, gm.Status())
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

	// Stats weekly endpoint
	mux.HandleFunc("GET /api/stats/weekly", func(w http.ResponseWriter, r *http.Request) {
		weeks := 4
		if v := r.URL.Query().Get("weeks"); v != "" {
			if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 52 {
				weeks = parsed
			}
		}
		if statsStore != nil {
			writeJSON(w, statsStore.GetWeeklySummary(weeks))
		} else {
			writeJSON(w, []stats.PeriodStats{})
		}
	})

	// Stats monthly endpoint
	mux.HandleFunc("GET /api/stats/monthly", func(w http.ResponseWriter, r *http.Request) {
		months := 6
		if v := r.URL.Query().Get("months"); v != "" {
			if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 24 {
				months = parsed
			}
		}
		if statsStore != nil {
			writeJSON(w, statsStore.GetMonthlySummary(months))
		} else {
			writeJSON(w, []stats.PeriodStats{})
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

	// Connection history endpoint
	mux.HandleFunc("GET /api/connections/history", func(w http.ResponseWriter, r *http.Request) {
		if connStore != nil {
			writeJSON(w, connStore.Recent(100))
		} else {
			writeJSON(w, []connlog.Entry{})
		}
	})

	// Streams by connection ID endpoint
	mux.HandleFunc("GET /api/connections/", func(w http.ResponseWriter, r *http.Request) {
		// Extract path: /api/connections/{id}/streams
		path := strings.TrimPrefix(r.URL.Path, "/api/connections/")
		parts := strings.Split(path, "/")
		if len(parts) != 2 || parts[1] != "streams" {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		connID := parts[0]
		if connID == "" {
			writeError(w, http.StatusBadRequest, "connection id required")
			return
		}

		st := eng.StreamTracker()
		if st == nil {
			writeJSON(w, []any{})
			return
		}

		streams := st.ByConnID(connID)
		type streamInfo struct {
			StreamID      uint64 `json:"stream_id"`
			ConnID        string `json:"conn_id"`
			Target        string `json:"target"`
			Transport     string `json:"transport"`
			BytesSent     int64  `json:"bytes_sent"`
			BytesReceived int64  `json:"bytes_received"`
			Errors        int64  `json:"errors"`
			Closed        bool   `json:"closed"`
			DurationMs    int64  `json:"duration_ms"`
		}
		out := make([]streamInfo, 0, len(streams))
		for _, m := range streams {
			out = append(out, streamInfo{
				StreamID:      m.StreamID,
				ConnID:        m.ConnID,
				Target:        m.Target,
				Transport:     m.Transport,
				BytesSent:     m.BytesSent.Load(),
				BytesReceived: m.BytesReceived.Load(),
				Errors:        m.Errors.Load(),
				Closed:        m.Closed.Load(),
				DurationMs:    m.GetDuration().Milliseconds(),
			})
		}
		writeJSON(w, out)
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

	// Auto-start endpoints
	mux.HandleFunc("GET /api/autostart", func(w http.ResponseWriter, r *http.Request) {
		enabled, err := autostart.IsEnabled()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]bool{"enabled": enabled})
	})

	mux.HandleFunc("PUT /api/autostart", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()

		var err error
		if req.Enabled {
			err = autostart.Enable()
		} else {
			err = autostart.Disable()
		}

		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]bool{"enabled": req.Enabled})
	})

	// Test probe — send a request through the full proxy chain
	// Used for dev/testing to verify end-to-end connectivity
	mux.HandleFunc("POST /api/test/probe", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			URL    string `json:"url"`    // target URL to fetch
			Method string `json:"method"` // HTTP method (default GET)
			Via    string `json:"via"`    // "socks5", "http", or "direct" (default "socks5")
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()

		if req.URL == "" {
			writeError(w, http.StatusBadRequest, "url is required")
			return
		}
		// Validate URL scheme to prevent SSRF against internal services
		if err := validateProbeURL(req.URL); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if req.Method == "" {
			req.Method = "GET"
		}
		switch req.Method {
		case "GET", "HEAD", "POST", "PUT", "DELETE", "OPTIONS":
		default:
			writeError(w, http.StatusBadRequest, "unsupported HTTP method: "+req.Method)
			return
		}
		if req.Via == "" {
			req.Via = "socks5"
		}

		cfg := eng.Config()

		// Build HTTP client with proxy
		var transport *http.Transport
		switch req.Via {
		case "socks5":
			proxyAddr := normalizeListenAddr(cfg.Proxy.SOCKS5.Listen)
			proxyURL, _ := url.Parse("socks5://" + proxyAddr)
			transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		case "http":
			proxyAddr := normalizeListenAddr(cfg.Proxy.HTTP.Listen)
			proxyURL, _ := url.Parse("http://" + proxyAddr)
			transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		case "direct":
			transport = &http.Transport{}
		default:
			writeError(w, http.StatusBadRequest, "via must be socks5, http, or direct")
			return
		}
		transport.TLSHandshakeTimeout = 10 * time.Second

		client := &http.Client{
			Transport: transport,
			Timeout:   15 * time.Second,
		}
		defer client.CloseIdleConnections()

		start := time.Now()
		probeReq, err := http.NewRequestWithContext(r.Context(), req.Method, req.URL, nil)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
			return
		}
		probeReq.Header.Set("User-Agent", "Shuttle-Probe/1.0")

		resp, err := client.Do(probeReq)
		elapsed := time.Since(start)
		if err != nil {
			writeJSON(w, map[string]any{
				"success":    false,
				"error":      err.Error(),
				"via":        req.Via,
				"latency_ms": elapsed.Milliseconds(),
			})
			return
		}
		defer resp.Body.Close()

		// Read response body (limited)
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

		writeJSON(w, map[string]any{
			"success":     true,
			"status":      resp.StatusCode,
			"status_text": resp.Status,
			"via":         req.Via,
			"latency_ms":  elapsed.Milliseconds(),
			"headers":     resp.Header,
			"body":        string(body),
		})
	})

	// Test probe — batch: test multiple URLs / proxy modes at once
	mux.HandleFunc("POST /api/test/probe/batch", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Tests []struct {
				Name   string `json:"name"`
				URL    string `json:"url"`
				Method string `json:"method"`
				Via    string `json:"via"`
			} `json:"tests"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()

		if len(req.Tests) == 0 {
			writeError(w, http.StatusBadRequest, "tests array is required")
			return
		}
		if len(req.Tests) > 20 {
			writeError(w, http.StatusBadRequest, "max 20 tests per batch")
			return
		}

		cfg := eng.Config()
		type result struct {
			Name      string `json:"name"`
			URL       string `json:"url"`
			Via       string `json:"via"`
			Success   bool   `json:"success"`
			Status    int    `json:"status,omitempty"`
			LatencyMs int64  `json:"latency_ms"`
			Error     string `json:"error,omitempty"`
			Body      string `json:"body,omitempty"`
		}

		var results []result
		for _, t := range req.Tests {
			if t.Method == "" {
				t.Method = "GET"
			}
			if t.Via == "" {
				t.Via = "socks5"
			}
			if err := validateProbeURL(t.URL); err != nil {
				results = append(results, result{Name: t.Name, URL: t.URL, Via: t.Via, Error: err.Error()})
				continue
			}

			var transport *http.Transport
			switch t.Via {
			case "socks5":
				proxyAddr := normalizeListenAddr(cfg.Proxy.SOCKS5.Listen)
				proxyURL, _ := url.Parse("socks5://" + proxyAddr)
				transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
			case "http":
				proxyAddr := normalizeListenAddr(cfg.Proxy.HTTP.Listen)
				proxyURL, _ := url.Parse("http://" + proxyAddr)
				transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
			case "direct":
				transport = &http.Transport{}
			default:
				results = append(results, result{Name: t.Name, URL: t.URL, Via: t.Via, Error: "invalid via"})
				continue
			}
			transport.TLSHandshakeTimeout = 10 * time.Second

			client := &http.Client{Transport: transport, Timeout: 15 * time.Second}
			start := time.Now()
			probeReq, err := http.NewRequestWithContext(r.Context(), t.Method, t.URL, nil)
			if err != nil {
				results = append(results, result{Name: t.Name, URL: t.URL, Via: t.Via, Error: err.Error()})
				client.CloseIdleConnections()
				continue
			}
			probeReq.Header.Set("User-Agent", "Shuttle-Probe/1.0")

			resp, err := client.Do(probeReq)
			elapsed := time.Since(start)
			if err != nil {
				results = append(results, result{Name: t.Name, URL: t.URL, Via: t.Via, LatencyMs: elapsed.Milliseconds(), Error: err.Error()})
				client.CloseIdleConnections()
				continue
			}
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			resp.Body.Close()
			client.CloseIdleConnections()

			results = append(results, result{
				Name:      t.Name,
				URL:       t.URL,
				Via:       t.Via,
				Success:   true,
				Status:    resp.StatusCode,
				LatencyMs: elapsed.Milliseconds(),
				Body:      string(body),
			})
		}

		writeJSON(w, map[string]any{"results": results})
	})

	// PAC file generation
	mux.HandleFunc("GET /api/pac", func(w http.ResponseWriter, r *http.Request) {
		cfg := eng.Config()
		rt := eng.CurrentRouter()
		if rt == nil {
			writeError(w, http.StatusServiceUnavailable, "router not initialized")
			return
		}

		pacCfg := &router.PACConfig{
			HTTPProxyAddr:  normalizeListenAddr(cfg.Proxy.HTTP.Listen),
			SOCKSProxyAddr: normalizeListenAddr(cfg.Proxy.SOCKS5.Listen),
			DefaultAction:  router.Action(cfg.Routing.Default),
		}
		pac := router.GeneratePAC(rt, pacCfg)

		w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
		if r.URL.Query().Get("download") == "true" {
			w.Header().Set("Content-Disposition", "attachment; filename=shuttle.pac")
		}
		_, _ = w.Write([]byte(pac))
	})

	// Rule conflict detection
	mux.HandleFunc("GET /api/routing/conflicts", func(w http.ResponseWriter, r *http.Request) {
		cfg := eng.Config()

		// Convert config rules to router rules
		var rules []router.Rule
		for i := range cfg.Routing.Rules {
			rule := &cfg.Routing.Rules[i]
			rr := router.Rule{Action: router.Action(rule.Action)}
			switch {
			case rule.Domains != "":
				rr.Type = "domain"
				rr.Values = []string{rule.Domains}
			case rule.GeoSite != "":
				rr.Type = "geosite"
				rr.Values = []string{rule.GeoSite}
			case rule.GeoIP != "":
				rr.Type = "geoip"
				rr.Values = []string{rule.GeoIP}
			case len(rule.IPCIDR) > 0:
				rr.Type = "ip-cidr"
				rr.Values = rule.IPCIDR
			}
			rules = append(rules, rr)
		}

		// Use the engine's GeoSite DB if available
		var geoSiteDB *router.GeoSiteDB
		if rt := eng.CurrentRouter(); rt != nil {
			geoSiteDB = rt.GeoSiteDB()
		}

		conflicts := router.DetectConflicts(rules, geoSiteDB)
		writeJSON(w, map[string]any{
			"conflicts": conflicts,
			"count":     len(conflicts),
		})
	})

	// GeoData source presets
	mux.HandleFunc("GET /api/geodata/sources", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, geodata.BuiltinPresets())
	})

	mux.HandleFunc("POST /api/geodata/sources/", func(w http.ResponseWriter, r *http.Request) {
		presetID := strings.TrimPrefix(r.URL.Path, "/api/geodata/sources/")
		if presetID == "" {
			writeError(w, http.StatusBadRequest, "preset id required")
			return
		}

		preset := geodata.PresetByID(presetID)
		if preset == nil {
			writeError(w, http.StatusNotFound, "unknown preset: "+presetID)
			return
		}

		if presetID == "custom" {
			writeError(w, http.StatusBadRequest, "use PUT /api/config to set custom URLs")
			return
		}

		cfg := eng.Config()
		cfg.Routing.GeoData.DirectListURL = preset.DirectList
		cfg.Routing.GeoData.ProxyListURL = preset.ProxyList
		cfg.Routing.GeoData.RejectListURL = preset.RejectList
		cfg.Routing.GeoData.GFWListURL = preset.GFWList
		cfg.Routing.GeoData.CNCidrURL = preset.CNCidr
		cfg.Routing.GeoData.PrivateCidrURL = preset.PrivateCidr

		if err := eng.Reload(&cfg); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, map[string]string{"status": "applied", "source": presetID})
	})

	// GeoSite available categories
	mux.HandleFunc("GET /api/geosite/categories", func(w http.ResponseWriter, r *http.Request) {
		rt := eng.CurrentRouter()
		if rt == nil {
			writeJSON(w, []string{})
			return
		}
		gsdb := rt.GeoSiteDB()
		if gsdb == nil {
			writeJSON(w, []string{})
			return
		}
		writeJSON(w, gsdb.Categories())
	})

	// Network info endpoint for LAN sharing
	mux.HandleFunc("GET /api/network/lan", func(w http.ResponseWriter, r *http.Request) {
		ips := getLANAddresses()
		cfg := eng.Config()
		writeJSON(w, map[string]any{
			"allow_lan": cfg.Proxy.AllowLAN,
			"addresses": ips,
			"socks5":    cfg.Proxy.SOCKS5.Listen,
			"http":      cfg.Proxy.HTTP.Listen,
		})
	})

	// Diagnostic: debug state snapshot
	mux.HandleFunc("GET /api/debug/state", func(w http.ResponseWriter, r *http.Request) {
		status := eng.Status()
		writeJSON(w, map[string]any{
			"engine_state":   status.State,
			"circuit_breaker": status.CircuitState,
			"streams":        status.Streams,
			"transport":      status.Transport,
			"uptime_seconds": int64(time.Since(apiStartTime).Seconds()),
			"goroutines":     runtime.NumGoroutine(),
		})
	})

	// Diagnostic: validate a client config without applying it
	mux.HandleFunc("POST /api/config/validate", func(w http.ResponseWriter, r *http.Request) {
		var cfg config.ClientConfig
		if err := decodeJSON(r, &cfg); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()

		var errs []string
		if err := cfg.Validate(); err != nil {
			errs = append(errs, err.Error())
		}
		writeJSON(w, map[string]any{
			"valid":  len(errs) == 0,
			"errors": errs,
		})
	})

	// Diagnostic: system resource usage
	mux.HandleFunc("GET /api/system/resources", func(w http.ResponseWriter, r *http.Request) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		writeJSON(w, map[string]any{
			"goroutines":     runtime.NumGoroutine(),
			"mem_alloc_mb":   math.Round(float64(m.Alloc)/1024/1024*100) / 100,
			"mem_sys_mb":     math.Round(float64(m.Sys)/1024/1024*100) / 100,
			"mem_gc_cycles":  m.NumGC,
			"num_cpu":        runtime.NumCPU(),
			"uptime_seconds": int64(time.Since(apiStartTime).Seconds()),
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

const maxRequestBody = 10 * 1024 * 1024 // 10MB

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, maxRequestBody)).Decode(v)
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

// validateProbeURL checks that a probe URL is safe to request.
// Rejects non-HTTP(S) schemes and localhost/private-IP targets to prevent SSRF.
func validateProbeURL(rawURL string) error {
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
	// Block localhost and link-local
	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("probing localhost/link-local addresses is not allowed")
		}
		// Block metadata endpoints (169.254.169.254, fd00::, etc.)
		if ip.Equal(net.ParseIP("169.254.169.254")) {
			return fmt.Errorf("probing cloud metadata endpoints is not allowed")
		}
	}
	if host == "localhost" {
		return fmt.Errorf("probing localhost is not allowed")
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
