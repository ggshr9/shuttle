package api

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/engine"
	"github.com/shuttleX/shuttle/router"
)

func registerRoutingRoutes(mux *http.ServeMux, eng *engine.Engine) {
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

	mux.HandleFunc("GET /api/routing/export", func(w http.ResponseWriter, r *http.Request) {
		cfg := eng.Config()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=shuttle-rules.json")
		writeJSON(w, cfg.Routing)
	})

	mux.HandleFunc("POST /api/routing/import", func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Rules   []config.RouteRule `json:"rules"`
			Default string             `json:"default"`
			Mode    string             `json:"mode"` // "merge" (default) or "replace"
		}
		if err := decodeJSON(r, &payload); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.Body.Close()

		cfg := eng.Config()
		existingCount := len(cfg.Routing.Rules)

		if payload.Mode == "replace" {
			cfg.Routing.Rules = payload.Rules
		} else {
			// Default: merge (append)
			cfg.Routing.Rules = append(cfg.Routing.Rules, payload.Rules...)
		}

		// Apply default action if specified in import
		if payload.Default != "" {
			cfg.Routing.Default = payload.Default
		}

		if err := eng.Reload(&cfg); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, map[string]any{
			"status":   "imported",
			"added":    len(payload.Rules),
			"total":    len(cfg.Routing.Rules),
			"existing": existingCount,
		})
	})

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
				routerCfg.Rules = append(routerCfg.Rules, router.ConfigRuleToRouterRule(rule))
			}
			rt = router.NewRouter(routerCfg, nil, nil, nil)
		}

		result := rt.DryRun(domain)
		writeJSON(w, result)
	})

	mux.HandleFunc("GET /api/routing/conflicts", func(w http.ResponseWriter, r *http.Request) {
		cfg := eng.Config()

		// Convert config rules to router rules
		var rules []router.Rule
		for _, rule := range cfg.Routing.Rules {
			rules = append(rules, router.ConfigRuleToRouterRule(rule))
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
}
