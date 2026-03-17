package api

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/engine"
	"github.com/shuttleX/shuttle/internal/procnet"
	"github.com/shuttleX/shuttle/router/geodata"
	"github.com/shuttleX/shuttle/subscription"
	"github.com/shuttleX/shuttle/update"
)

func registerMiscRoutes(mux *http.ServeMux, eng *engine.Engine, subMgr *subscription.Manager, updateChecker *update.Checker) {
	// Full backup - exports complete configuration including subscriptions
	mux.HandleFunc("GET /api/backup", func(w http.ResponseWriter, r *http.Request) {
		cfg := eng.Config()

		// Redact secrets unless explicitly requested
		if r.URL.Query().Get("include_secrets") != "true" {
			redactClientConfig(&cfg)
		}

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
		writeJSON(w, backup)
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
				if _, err := subMgr.Add(sub.Name, sub.URL); err != nil {
					slog.Warn("restore: failed to add subscription", "name", sub.Name, "url", sub.URL, "err", err)
				}
			}
		}

		// Restore configuration
		if err := eng.Reload(&backup.Config); err != nil {
			writeError(w, http.StatusInternalServerError, "restore failed: "+err.Error())
			return
		}

		writeJSON(w, map[string]interface{}{
			"status":        "restored",
			"servers":       len(backup.Config.Servers),
			"subscriptions": len(backup.Subscriptions),
		})
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

	// Log export endpoint
	mux.HandleFunc("GET /api/logs/export", func(w http.ResponseWriter, r *http.Request) {
		cfg := eng.Config()
		status := eng.Status()

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=shuttle-logs.txt")

		_, _ = w.Write([]byte("Shuttle Log Export\n"))
		_, _ = w.Write([]byte("==================\n\n"))
		_, _ = w.Write([]byte("Engine Status:\n"))
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

	// Diagnostics bundle
	mux.HandleFunc("GET /api/diagnostics", func(w http.ResponseWriter, r *http.Request) {
		ts := time.Now().Format("20060102-150405")
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="shuttle-diagnostics-%s.zip"`, ts))
		if err := eng.ExportDiagnosticsZIP(w); err != nil {
			// Headers already sent, log error
			_ = err
		}
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

	// Process list
	mux.HandleFunc("GET /api/processes", func(w http.ResponseWriter, r *http.Request) {
		procs, err := procnet.ListNetworkProcesses()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, procs)
	})

	// Test probe — send a request through the full proxy chain
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

	// Test probe — batch
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
}
