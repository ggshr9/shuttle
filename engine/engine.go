package engine

import (
	"log/slog"
	"sync"

	"github.com/shuttleX/shuttle/adapter"
	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/internal/logutil"
	"github.com/shuttleX/shuttle/internal/netmon"
	"github.com/shuttleX/shuttle/mesh"
	"github.com/shuttleX/shuttle/plugin"
	"github.com/shuttleX/shuttle/router"
	"github.com/shuttleX/shuttle/router/geodata"
	"github.com/shuttleX/shuttle/stream"
	"github.com/shuttleX/shuttle/transport/selector"

	"context"
	"fmt"
)

// Engine is the core shuttle client, managing transports, routing, and local proxies.
type Engine struct {
	mu      sync.RWMutex
	state   EngineState
	cfg     *config.ClientConfig
	logger  *slog.Logger
	metrics *plugin.Metrics

	// lifecycleMu serialises Start/Stop/Reload so that concurrent callers
	// cannot interleave their long-running init/shutdown sequences.
	lifecycleMu sync.Mutex

	sel       *selector.Selector
	cancel    context.CancelFunc
	parentCtx context.Context // stored for Reload

	// Closers for local proxy servers
	closers []func() error

	// Mesh client for P2P VPN
	meshClient *mesh.MeshClient

	// Network change monitor
	netMon *netmon.Monitor

	// Geo data manager
	geoManager *geodata.Manager

	// Current router (for PAC generation and conflict detection)
	currentRouter *router.Router

	// DNS resolver built alongside the router
	dnsResolver *router.DNSResolver

	// Stream-level metrics tracker
	streamTracker *stream.StreamTracker

	// Circuit breaker for transport connections
	circuitBreaker *CircuitBreaker

	// Plugin chain for connection tracking, filtering, and logging
	chain *plugin.Chain

	// Background goroutine tracking for clean shutdown
	bgWg sync.WaitGroup

	// Connection sequence counter for generating correlation IDs
	connSeq uint64

	// Inbound/outbound abstraction layer
	inbounds  []adapter.Inbound
	outbounds map[string]adapter.Outbound

	// Event subscribers — stores bidirectional channels, Subscribe returns receive-only view
	subMu sync.RWMutex
	subs  map[chan Event]struct{}
}

// New creates a new Engine from the given config.
func New(cfg *config.ClientConfig) *Engine {
	logger := logutil.NewLogger(cfg.Log.Level, cfg.Log.Format)

	return &Engine{
		state:   StateStopped,
		cfg:     cfg,
		logger:  logger,
		metrics: plugin.NewMetrics(),
		subs:    make(map[chan Event]struct{}),
	}
}

// selector returns the current selector under read lock.
func (e *Engine) selector() *selector.Selector {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.sel
}

// ValidateConfig checks whether a config can start an engine
// (at least one transport enabled, server addr set).
func ValidateConfig(cfg *config.ClientConfig) error {
	if cfg.Server.Addr == "" {
		return fmt.Errorf("server address is required")
	}
	if !cfg.Transport.H3.Enabled && !cfg.Transport.Reality.Enabled && !cfg.Transport.CDN.Enabled && !cfg.Transport.WebRTC.Enabled {
		return fmt.Errorf("at least one transport must be enabled")
	}
	return nil
}

// Status returns a snapshot of the engine's current state.
func (e *Engine) Status() EngineStatus {
	e.mu.RLock()
	state := e.state
	sel := e.sel
	mc := e.meshClient
	cfg := e.cfg
	st := e.streamTracker
	cb := e.circuitBreaker
	e.mu.RUnlock()

	stats := e.metrics.Stats()
	up, down := e.metrics.Speed()

	status := EngineStatus{
		State:         state.String(),
		ActiveConns:   stats["active_conns"],
		TotalConns:    stats["total_conns"],
		BytesSent:     stats["bytes_sent"],
		BytesReceived: stats["bytes_received"],
		UploadSpeed:   up,
		DownloadSpeed: down,
	}

	if cb != nil {
		status.CircuitState = cb.State().String()
	}

	// Add stream-level metrics summary.
	if st != nil {
		sum := st.Summary()
		status.Streams = &StreamStats{
			TotalStreams:   sum.TotalStreams,
			ActiveStreams:  sum.ActiveStreams,
			TotalBytesSent: sum.TotalBytesSent,
			TotalBytesRecv: sum.TotalBytesRecv,
			AvgDurationMs:  sum.AvgDuration.Milliseconds(),
		}
		status.TransportBreakdown = st.ByTransport()
	}

	if sel != nil {
		status.Transport = sel.ActiveTransport()
		for typ, probe := range sel.Probes() {
			status.Transports = append(status.Transports, TransportInfo{
				Type:      typ,
				Available: probe.Available,
				Latency:   probe.Latency.Milliseconds(),
			})
		}
		if paths := sel.ActivePaths(); len(paths) > 0 {
			for _, sp := range paths {
				status.MultipathPaths = append(status.MultipathPaths, PathInfo{
					Transport:     sp.Transport,
					Latency:       sp.Latency,
					ActiveStreams: sp.ActiveStreams,
					TotalStreams:  sp.TotalStreams,
					Available:     sp.Available,
					Failures:      sp.Failures,
					BytesSent:     sp.BytesSent,
					BytesReceived: sp.BytesReceived,
				})
			}
		}
	}

	// Add mesh status
	if cfg != nil && cfg.Mesh.Enabled {
		meshStatus := &MeshStatus{Enabled: true}
		if mc != nil {
			meshStatus.VirtualIP = mc.VirtualIP().String()
			meshStatus.CIDR = mc.MeshCIDR()
			for _, peer := range mc.ListPeers() {
				mp := MeshPeer{
					VirtualIP: peer.VirtualIP,
					State:     peer.State,
					Method:    peer.Method,
				}
				if peer.Quality != nil {
					mp.AvgRTT = peer.Quality.AvgRTT.Milliseconds()
					mp.MinRTT = peer.Quality.MinRTT.Milliseconds()
					mp.MaxRTT = peer.Quality.MaxRTT.Milliseconds()
					mp.Jitter = peer.Quality.Jitter.Milliseconds()
					mp.PacketLoss = peer.Quality.LossRate
					mp.Score = peer.Quality.Score
				}
				meshStatus.Peers = append(meshStatus.Peers, mp)
			}
		}
		status.Mesh = meshStatus
	}

	return status
}

// StreamStats returns an aggregate summary of per-stream metrics.
func (e *Engine) StreamStats() stream.StreamSummary {
	e.mu.RLock()
	st := e.streamTracker
	e.mu.RUnlock()
	if st == nil {
		return stream.StreamSummary{}
	}
	return st.Summary()
}

// MultipathStats returns per-path statistics from any multipath-capable transport, or nil.
func (e *Engine) MultipathStats() []adapter.MultipathStats {
	e.mu.RLock()
	sel := e.sel
	e.mu.RUnlock()
	if sel == nil {
		return nil
	}
	for _, t := range sel.Transports() {
		if mp, ok := t.(adapter.MultipathStatsProvider); ok {
			if stats := mp.MultipathStats(); stats != nil {
				return stats
			}
		}
	}
	return nil
}

// StreamTracker returns the current stream tracker, or nil if the engine is not running.
func (e *Engine) StreamTracker() *stream.StreamTracker {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.streamTracker
}

// Config returns a deep copy of the current config.
func (e *Engine) Config() config.ClientConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return *e.cfg.DeepCopy()
}

// SetConfig updates the config without restarting the engine.
// Use this for non-critical changes like the saved server list.
func (e *Engine) SetConfig(cfg *config.ClientConfig) {
	cp := cfg.DeepCopy()
	e.mu.Lock()
	e.cfg = cp
	e.mu.Unlock()
}

// GeoManager returns the geo data manager, or nil if not enabled.
func (e *Engine) GeoManager() *geodata.Manager {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.geoManager
}

// CurrentRouter returns the active router, or nil if not running.
func (e *Engine) CurrentRouter() *router.Router {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.currentRouter
}
