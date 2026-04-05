package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"sync"
	"time"

	"github.com/shuttleX/shuttle/adapter"
	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/congestion"
	"github.com/shuttleX/shuttle/mesh"
	meshsignal "github.com/shuttleX/shuttle/mesh/signal"
	"github.com/shuttleX/shuttle/server/admin"
	"github.com/shuttleX/shuttle/server/audit"
	"github.com/shuttleX/shuttle/server/metrics"
)

// Config holds everything the Server needs to initialize.
type Config struct {
	ServerConfig *config.ServerConfig
	ConfigPath   string
	Version      string
	Logger       *slog.Logger
}

// Server encapsulates the entire shuttled server lifecycle: transport
// listeners, mesh, admin API, cluster, reputation, audit, metrics, pprof.
type Server struct {
	cfg        *config.ServerConfig
	configPath string
	version    string
	logger     *slog.Logger

	ml          *MultiListener
	handler     *Handler
	adminInfo   *admin.ServerInfo
	adminServer *http.Server
	pprofServer *http.Server
	cluster     *ClusterManager
	reputation  *Reputation
	auditLog    *audit.Logger
	metrics     *metrics.Collector
	users       *admin.UserStore
	peerTable   *mesh.PeerTable
	ipAllocator *mesh.IPAllocator
	signalHub   *meshsignal.Hub

	connWg sync.WaitGroup

	// reputationCancel stops the reputation cleanup goroutine.
	reputationCancel context.CancelFunc
}

// New creates a Server, initializing all subsystems according to cfg.
// It does not start any listeners; call Start for that.
func New(c Config) (*Server, error) {
	cfg := c.ServerConfig
	logger := c.Logger
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		cfg:        cfg,
		configPath: c.ConfigPath,
		version:    c.Version,
		logger:     logger,
		metrics:    metrics.NewCollector(),
	}

	// --- Pprof ---
	if cfg.Debug.PprofEnabled {
		pprofListen := cfg.Debug.PprofListen
		if pprofListen == "" {
			pprofListen = "127.0.0.1:6060"
		}
		pprofMux := http.NewServeMux()
		pprofMux.HandleFunc("/debug/pprof/", pprof.Index)
		pprofMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		pprofMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		pprofMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		pprofMux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		s.pprofServer = &http.Server{Addr: pprofListen, Handler: pprofMux}
	}

	// --- Congestion control ---
	adaptive := congestion.NewAdaptive(nil, logger)
	ccAdapter := congestion.NewQUICAdapter(adaptive)

	// --- Multi-listener + transports ---
	s.ml = NewMultiListener(&ListenerConfig{
		ListenAddr: cfg.Listen,
	}, logger)

	opts := adapter.FactoryOptions{
		Logger:            logger,
		CongestionControl: ccAdapter,
	}
	for name, factory := range adapter.All() {
		t, err := factory.NewServer(cfg, opts)
		if err != nil {
			s.logger.Warn("transport factory failed", "type", name, "err", err)
			continue
		}
		if t != nil {
			s.ml.AddTransport(t)
		}
	}

	// --- Mesh ---
	if cfg.Mesh.Enabled {
		var err error
		s.ipAllocator, err = mesh.NewIPAllocator(cfg.Mesh.CIDR)
		if err != nil {
			return nil, err
		}
		s.peerTable = mesh.NewPeerTable(logger)
		logger.Info("mesh enabled", "cidr", cfg.Mesh.CIDR)

		if cfg.Mesh.P2PEnabled {
			s.signalHub = meshsignal.NewHub(logger)
			logger.Info("mesh P2P signaling enabled")
		}
	}

	// --- IP reputation ---
	if cfg.Reputation.Enabled {
		maxFailures := cfg.Reputation.MaxFailures
		if maxFailures <= 0 {
			maxFailures = 5
		}
		s.reputation = NewReputation(ReputationConfig{
			Enabled:     true,
			MaxFailures: maxFailures,
		})
		logger.Info("IP reputation tracking enabled", "max_failures", maxFailures)
	}

	// --- Users ---
	s.users = admin.NewUserStore(cfg.Admin.Users)

	// --- Audit ---
	if cfg.Audit.Enabled {
		var err error
		s.auditLog, err = audit.NewLogger(cfg.Audit.LogDir, cfg.Audit.MaxEntries)
		if err != nil {
			logger.Error("failed to create audit logger", "err", err)
		} else {
			logger.Info("audit logging enabled", "log_dir", cfg.Audit.LogDir)
		}
	}

	// --- Admin API ---
	if cfg.Admin.Enabled {
		s.adminInfo = &admin.ServerInfo{
			StartTime:  time.Now(),
			Version:    c.Version,
			ConfigPath: c.ConfigPath,
		}
		var err error
		s.adminServer, err = admin.ListenAndServe(&cfg.Admin, s.adminInfo, cfg, c.ConfigPath, s.users, s.auditLog, s.metrics)
		if err != nil {
			logger.Error("failed to start admin API", "err", err)
		} else {
			logger.Info("admin API listening", "addr", cfg.Admin.Listen)
		}
	}

	// --- Cluster ---
	if cfg.Cluster.Enabled {
		clusterPeers := make([]PeerConfig, len(cfg.Cluster.Peers))
		for i, p := range cfg.Cluster.Peers {
			clusterPeers[i] = PeerConfig{Name: p.Name, Addr: p.Addr}
		}
		clusterInfo := &ClusterNodeInfo{Version: c.Version}
		if s.adminInfo != nil {
			clusterInfo.ActiveConns = &s.adminInfo.ActiveConns
			clusterInfo.TotalConns = &s.adminInfo.TotalConns
			clusterInfo.BytesSent = &s.adminInfo.BytesSent
			clusterInfo.BytesRecv = &s.adminInfo.BytesRecv
		}
		s.cluster = NewClusterManager(&ClusterConfig{
			Enabled:  true,
			NodeName: cfg.Cluster.NodeName,
			Secret:   cfg.Cluster.Secret,
			Peers:    clusterPeers,
			Interval: cfg.Cluster.Interval,
			MaxConns: cfg.Cluster.MaxConns,
		}, clusterInfo, logger)
	}

	// --- Connection handler ---
	s.handler = &Handler{
		Users:      s.users,
		Reputation: s.reputation,
		AuditLog:   s.auditLog,
		PeerTable:  s.peerTable,
		Allocator:  s.ipAllocator,
		SignalHub:  s.signalHub,
		Metrics:    s.metrics,
		AdminInfo:  s.adminInfo,
		StreamSem:  make(chan struct{}, cfg.MaxStreams),
		Logger:     logger,
	}

	return s, nil
}

// Start begins listening and accepting connections. It blocks until
// ctx is cancelled or an unrecoverable error occurs.
func (s *Server) Start(ctx context.Context) error {
	// Start pprof
	if s.pprofServer != nil {
		go func() {
			s.logger.Info("pprof enabled", "addr", s.pprofServer.Addr)
			if err := s.pprofServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				s.logger.Error("pprof server failed", "err", err)
			}
		}()
	}

	// Start reputation cleanup
	if s.reputation != nil {
		repCtx, repCancel := context.WithCancel(ctx)
		s.reputationCancel = repCancel
		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-repCtx.Done():
					return
				case <-ticker.C:
					s.reputation.Cleanup()
				}
			}
		}()
	}

	// Start cluster
	if s.cluster != nil {
		s.cluster.Start(ctx)
		s.logger.Info("cluster enabled",
			"node", s.cfg.Cluster.NodeName,
			"peers", len(s.cfg.Cluster.Peers))
	}

	// Start transport listeners
	if err := s.ml.Start(ctx); err != nil {
		return err
	}

	s.logger.Info("shuttled is running", "listen", s.cfg.Listen)

	// Accept loop
	for {
		conn, err := s.ml.Accept(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			s.logger.Error("accept error", "err", err)
			continue
		}

		// Check IP reputation
		if s.reputation != nil {
			remoteIP := ExtractIP(conn.RemoteAddr())
			if s.reputation.IsBanned(remoteIP) {
				s.logger.Debug("rejecting banned IP", "ip", remoteIP)
				conn.Close()
				continue
			}
		}

		if s.adminInfo != nil {
			s.adminInfo.TotalConns.Add(1)
			s.adminInfo.ActiveConns.Add(1)
		}

		transportName := "unknown"
		if tn, ok := conn.(interface{ TransportName() string }); ok {
			transportName = tn.TransportName()
		}
		s.metrics.ConnOpened(transportName)
		connStart := time.Now()

		s.connWg.Add(1)
		go func() {
			defer s.connWg.Done()
			s.handler.HandleConnection(ctx, conn)
			if s.adminInfo != nil {
				s.adminInfo.ActiveConns.Add(-1)
			}
			s.metrics.ConnClosed(transportName, time.Since(connStart))
		}()
	}
}

// Shutdown performs graceful shutdown: stops accepting, drains connections,
// and closes all subsystems. The provided ctx controls the drain timeout.
func (s *Server) Shutdown(ctx context.Context) {
	// Phase 1: Stop accepting new connections
	s.ml.Close()

	// Wait for active connections to finish or drain timeout
	drainDone := make(chan struct{})
	go func() {
		s.connWg.Wait()
		close(drainDone)
	}()

	select {
	case <-drainDone:
		s.logger.Info("all connections drained")
	case <-ctx.Done():
		s.logger.Warn("drain timeout or forced shutdown, closing remaining connections")
	}

	// Stop reputation cleanup
	if s.reputationCancel != nil {
		s.reputationCancel()
	}

	// Shut down pprof
	if s.pprofServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := s.pprofServer.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("pprof server shutdown error", "err", err)
		}
		shutdownCancel()
	}

	// Shut down admin server
	if s.adminServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := s.adminServer.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("admin server shutdown error", "err", err)
		}
		shutdownCancel()
	}

	// Stop cluster manager
	if s.cluster != nil {
		s.cluster.Stop()
	}

	// Close audit logger
	if s.auditLog != nil {
		s.auditLog.Close()
	}
}
