package api

import (
	"io/fs"
	"net"
	"net/http"
	"strings"

	"github.com/shuttle-proxy/shuttle/engine"
)

// Server wraps the API handler with SPA fallback serving.
type Server struct {
	eng      *engine.Engine
	listener net.Listener
	srv      *http.Server
}

// NewServer creates an API server. If webFS is non-nil, it serves the SPA from it
// with fallback to index.html for client-side routing.
func NewServer(eng *engine.Engine, webFS fs.FS) *Server {
	apiHandler := Handler(eng)

	var handler http.Handler
	if webFS != nil {
		fileServer := http.FileServer(http.FS(webFS))
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// API routes go to the API handler
			if strings.HasPrefix(r.URL.Path, "/api/") {
				apiHandler.ServeHTTP(w, r)
				return
			}
			// Try to serve static file
			f, err := webFS.Open(strings.TrimPrefix(r.URL.Path, "/"))
			if err != nil {
				// SPA fallback: serve index.html for unknown paths
				r.URL.Path = "/"
				fileServer.ServeHTTP(w, r)
				return
			}
			f.Close()
			fileServer.ServeHTTP(w, r)
		})
	} else {
		handler = apiHandler
	}

	return &Server{
		eng: eng,
		srv: &http.Server{Handler: handler},
	}
}

// ListenAndServe starts the server on the given address.
// Use ":0" for a random port. Returns the actual address after binding.
func (s *Server) ListenAndServe(addr string) (string, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	s.listener = ln
	go s.srv.Serve(ln)
	return ln.Addr().String(), nil
}

// Close shuts down the server.
func (s *Server) Close() error {
	return s.srv.Close()
}
