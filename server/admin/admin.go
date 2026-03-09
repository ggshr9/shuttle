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
	"sync/atomic"
	"time"

	"github.com/shuttle-proxy/shuttle/config"
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
func Handler(info *ServerInfo, cfg *config.ServerConfig, configPath string) http.Handler {
	mux := http.NewServeMux()
	token := cfg.Admin.Token

	auth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			provided := r.Header.Get("Authorization")
			expected := "Bearer " + token
			if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
				writeError(w, http.StatusUnauthorized, "invalid or missing token")
				return
			}
			next(w, r)
		}
	}

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
		redacted := *cfg
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
		*cfg = *newCfg
		writeJSON(w, map[string]string{"status": "reloaded"})
	}))

	return mux
}

// ListenAndServe starts the admin API server.
func ListenAndServe(cfg *config.AdminConfig, info *ServerInfo, serverCfg *config.ServerConfig, configPath string) error {
	if !cfg.Enabled {
		return nil
	}

	listen := cfg.Listen
	if listen == "" {
		listen = "127.0.0.1:9090"
	}

	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return fmt.Errorf("admin listen: %w", err)
	}

	handler := Handler(info, serverCfg, configPath)
	server := &http.Server{
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go server.Serve(ln)
	return nil
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
