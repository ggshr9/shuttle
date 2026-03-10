// Package admin provides a REST API for remote server management.
// Designed for AI assistants (OpenClaw, etc.) and automation tools.
package admin

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shuttle-proxy/shuttle/config"
	"github.com/shuttle-proxy/shuttle/server/audit"
)

// ServerInfo tracks runtime server metrics.
type ServerInfo struct {
	StartTime   time.Time
	Version     string
	ConfigPath  string
	ActiveConns atomic.Int64
	TotalConns  atomic.Int64
	BytesSent   atomic.Int64
	BytesRecv   atomic.Int64
}

// Handler creates the admin API HTTP handler.
func Handler(info *ServerInfo, cfg *config.ServerConfig, configPath string, users *UserStore, auditLog *audit.Logger) http.Handler {
	mux := http.NewServeMux()
	var cfgMu sync.RWMutex // protects concurrent access to cfg
	token := cfg.Admin.Token

	// Rate limiter: 1 token/sec refill, burst of 5.
	limiter := NewRateLimiter(1, 5)
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			limiter.cleanup()
		}
	}()

	auth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ip := extractIP(r.RemoteAddr)
			if !limiter.Allow(ip) {
				writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}
			if token == "" {
				writeError(w, http.StatusForbidden, "admin token not configured")
				return
			}
			provided := r.Header.Get("Authorization")
			expected := "Bearer " + token
			if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
				writeError(w, http.StatusUnauthorized, "invalid or missing token")
				return
			}
			next(w, r)
		}
	}

	// Dashboard — no auth required (login handled client-side)
	mux.HandleFunc("GET /", handleDashboard)

	// Health check — no auth required
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /api/status", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"version":      info.Version,
			"uptime":       time.Since(info.StartTime).String(),
			"uptime_secs":  int(time.Since(info.StartTime).Seconds()),
			"active_conns": info.ActiveConns.Load(),
			"total_conns":  info.TotalConns.Load(),
			"bytes_sent":   info.BytesSent.Load(),
			"bytes_recv":   info.BytesRecv.Load(),
			"go_version":   runtime.Version(),
			"os":           runtime.GOOS,
			"arch":         runtime.GOARCH,
		})
	}))

	mux.HandleFunc("GET /api/config", auth(func(w http.ResponseWriter, r *http.Request) {
		// Return config with secrets redacted
		cfgMu.RLock()
		redacted := *cfg
		cfgMu.RUnlock()
		redacted.Auth.Password = "***"
		redacted.Auth.PrivateKey = "***"
		redacted.Admin.Token = "***"
		writeJSON(w, &redacted)
	}))

	mux.HandleFunc("GET /api/share", auth(func(w http.ResponseWriter, r *http.Request) {
		addr := r.URL.Query().Get("addr")
		if addr == "" {
			addr = cfg.Listen
		}
		name := r.URL.Query().Get("name")

		s := &config.ShareURI{
			Addr:     addr,
			Password: cfg.Auth.Password,
			Name:     name,
		}

		h3Enabled := cfg.Transport.H3.Enabled
		realityEnabled := cfg.Transport.Reality.Enabled
		switch {
		case h3Enabled && realityEnabled:
			s.Transport = "both"
		case h3Enabled:
			s.Transport = "h3"
		case realityEnabled:
			s.Transport = "reality"
		}

		if realityEnabled {
			s.PublicKey = cfg.Auth.PublicKey
			s.SNI = cfg.Transport.Reality.TargetSNI
			if len(cfg.Transport.Reality.ShortIDs) > 0 {
				s.ShortID = cfg.Transport.Reality.ShortIDs[0]
			}
		}

		uri := config.EncodeShareURI(s)
		writeJSON(w, map[string]string{
			"uri":      uri,
			"password": cfg.Auth.Password,
		})
	}))

	mux.HandleFunc("GET /api/metrics", auth(func(w http.ResponseWriter, r *http.Request) {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		writeJSON(w, map[string]any{
			"active_conns": info.ActiveConns.Load(),
			"total_conns":  info.TotalConns.Load(),
			"bytes_sent":   info.BytesSent.Load(),
			"bytes_recv":   info.BytesRecv.Load(),
			"mem_alloc":    mem.Alloc,
			"mem_sys":      mem.Sys,
			"goroutines":   runtime.NumGoroutine(),
		})
	}))

	mux.HandleFunc("GET /metrics", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		WritePrometheusMetrics(w, info, users)
	}))

	mux.HandleFunc("POST /api/reload", auth(func(w http.ResponseWriter, r *http.Request) {
		if configPath == "" {
			writeError(w, http.StatusBadRequest, "no config path available for reload")
			return
		}
		newCfg, err := config.LoadServerConfig(configPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("reload failed: %v", err))
			return
		}
		// Update the in-memory config reference
		cfgMu.Lock()
		*cfg = *newCfg
		cfgMu.Unlock()
		writeJSON(w, map[string]string{"status": "reloaded"})
	}))

	// User management
	mux.HandleFunc("GET /api/users", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, users.List())
	}))

	mux.HandleFunc("POST /api/users", auth(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name     string `json:"name"`
			MaxBytes int64  `json:"max_bytes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		user, err := users.Add(req.Name, req.MaxBytes)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Sync back to config
		cfgMu.Lock()
		cfg.Admin.Users = users.ToConfig()
		cfgMu.Unlock()
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, user)
	}))

	mux.HandleFunc("DELETE /api/users/", auth(func(w http.ResponseWriter, r *http.Request) {
		userToken := r.URL.Path[len("/api/users/"):]
		if userToken == "" {
			writeError(w, http.StatusBadRequest, "user token required")
			return
		}
		if !users.Remove(userToken) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		cfgMu.Lock()
		cfg.Admin.Users = users.ToConfig()
		cfgMu.Unlock()
		writeJSON(w, map[string]string{"status": "deleted"})
	}))

	mux.HandleFunc("PUT /api/users/", auth(func(w http.ResponseWriter, r *http.Request) {
		userToken := r.URL.Path[len("/api/users/"):]
		if userToken == "" {
			writeError(w, http.StatusBadRequest, "user token required")
			return
		}
		var req struct {
			Enabled *bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.Enabled != nil {
			if !users.SetEnabled(userToken, *req.Enabled) {
				writeError(w, http.StatusNotFound, "user not found")
				return
			}
		}
		cfgMu.Lock()
		cfg.Admin.Users = users.ToConfig()
		cfgMu.Unlock()
		writeJSON(w, map[string]string{"status": "updated"})
	}))

	mux.HandleFunc("GET /api/audit", auth(func(w http.ResponseWriter, r *http.Request) {
		if auditLog == nil {
			writeJSON(w, []audit.Entry{})
			return
		}
		n := 100
		if s := r.URL.Query().Get("n"); s != "" {
			if parsed, err := strconv.Atoi(s); err == nil && parsed > 0 {
				n = parsed
			}
		}
		writeJSON(w, auditLog.Recent(n))
	}))

	return mux
}

// ListenAndServe starts the admin API server and returns the *http.Server
// so the caller can call Shutdown() for graceful termination.
// If the admin API is disabled, it returns (nil, nil).
func ListenAndServe(cfg *config.AdminConfig, info *ServerInfo, serverCfg *config.ServerConfig, configPath string, users *UserStore, auditLog *audit.Logger) (*http.Server, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	listen := cfg.Listen
	if listen == "" {
		listen = "127.0.0.1:9090"
	}

	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return nil, fmt.Errorf("admin listen: %w", err)
	}

	handler := Handler(info, serverCfg, configPath, users, auditLog)
	server := &http.Server{
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go server.Serve(ln)
	return server, nil
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
