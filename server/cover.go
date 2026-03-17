package server

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
)

// CoverConfig configures the cover website.
type CoverConfig struct {
	// Mode: "static" serves local files, "reverse" proxies to another site
	Mode       string
	StaticDir  string // Directory for static mode
	ReverseURL string // URL for reverse proxy mode
}

// NewCoverHandler creates an HTTP handler that serves cover website content.
// This makes the server appear as a legitimate website to active probers.
func NewCoverHandler(cfg *CoverConfig, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	switch cfg.Mode {
	case "reverse":
		return newReverseProxy(cfg.ReverseURL, logger)
	case "static":
		return newStaticHandler(cfg.StaticDir, logger)
	default:
		// Default: serve a minimal placeholder page
		return newDefaultHandler()
	}
}

func newReverseProxy(target string, logger *slog.Logger) http.Handler {
	u, err := url.Parse(target)
	if err != nil {
		logger.Error("invalid reverse proxy URL", "url", target, "err", err)
		return newDefaultHandler()
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Debug("reverse proxy error", "err", err)
		w.WriteHeader(http.StatusBadGateway)
	}
	return proxy
}

func newStaticHandler(dir string, logger *slog.Logger) http.Handler {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		logger.Error("invalid static dir", "dir", dir, "err", err)
		return newDefaultHandler()
	}
	if _, err := os.Stat(absDir); os.IsNotExist(err) {
		logger.Error("static dir does not exist", "dir", absDir)
		return newDefaultHandler()
	}
	return http.FileServer(http.Dir(absDir))
}

func newDefaultHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Server", "nginx/1.24.0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>Welcome</title></head>
<body>
<h1>Welcome to our website</h1>
<p>This site is under construction. Please check back later.</p>
</body>
</html>`))
	})
}
