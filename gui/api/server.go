package api

import (
	"context"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/shuttleX/shuttle/engine"
	"github.com/shuttleX/shuttle/internal/healthcheck"
)

// Server wraps the API handler with SPA fallback serving.
type Server struct {
	eng        *engine.Engine
	listener   net.Listener
	srv        *http.Server
	pumpStop   context.CancelFunc // nil if no engine
	hbStop     chan struct{}      // closes when server shuts down to stop heartbeat
	hbStopOnce sync.Once          // guards hbStop close — concurrent Close() calls are safe
}

// NewServer creates an API server. If webFS is non-nil, it serves the SPA from it
// with fallback to index.html for client-side routing.
//
// An EventQueue is created and wired to the engine event bus so that
// /api/events and /ws/events have live data to serve.
func NewServer(eng *engine.Engine, webFS fs.FS) *Server {
	q := NewEventQueue(1024)
	var pumpStop context.CancelFunc
	if eng != nil {
		var pumpCtx context.Context
		pumpCtx, pumpStop = context.WithCancel(context.Background())
		go pumpEngineEvents(pumpCtx, eng, q)
	}

	// Liveness heartbeat — ticked every 5s so /api/health/live can detect
	// a hung event-loop / locked process (it goes stale after the
	// 30s threshold defined in health_deep.go). Mirrors the admin server
	// pattern in server/server.go.
	hb := healthcheck.NewHeartbeat()
	hbStop := make(chan struct{})
	hb.Run(hbStop, 5*time.Second)

	handler := NewHandler(HandlerConfig{Engine: eng, Events: q, Heartbeat: hb})
	srv := NewServerWithHandler(eng, webFS, handler)
	srv.pumpStop = pumpStop
	srv.hbStop = hbStop
	return srv
}

// pumpEngineEvents subscribes to the engine event bus and forwards every event
// into the EventQueue. Exits when ctx is canceled OR the engine subscriber
// channel closes (engine shutdown).
func pumpEngineEvents(ctx context.Context, eng *engine.Engine, q *EventQueue) {
	ch := eng.Subscribe()
	defer eng.Unsubscribe(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			q.Push(ev.Type.String(), ev)
		}
	}
}

// NewServerWithHandler creates an API server using a pre-built API handler.
// This allows sharing the same handler (with stats, connlog, subscriptions)
// between the standalone server and Wails asset handler.
func NewServerWithHandler(eng *engine.Engine, webFS fs.FS, apiHandler http.Handler) *Server {
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
			path := strings.TrimPrefix(r.URL.Path, "/")
			f, err := webFS.Open(path)
			if err != nil {
				// SPA fallback: serve index.html only for paths without
				// a file extension (i.e. client-side routes, not missing assets).
				if !strings.Contains(path[strings.LastIndex(path, "/")+1:], ".") {
					r.URL.Path = "/"
					fileServer.ServeHTTP(w, r)
					return
				}
				http.NotFound(w, r)
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
	go func() { _ = s.srv.Serve(ln) }()
	return ln.Addr().String(), nil
}

// Close shuts down the server. Safe to call concurrently — the heartbeat
// stop channel is closed exactly once via sync.Once.
func (s *Server) Close() error {
	if s.pumpStop != nil {
		s.pumpStop()
	}
	if s.hbStop != nil {
		s.hbStopOnce.Do(func() { close(s.hbStop) })
	}
	return s.srv.Close()
}
