package engine

import (
	"log/slog"
	"sync"

	"github.com/ggshr9/shuttle/adapter"
	"github.com/ggshr9/shuttle/config"
	"github.com/ggshr9/shuttle/internal/logutil"
	"github.com/ggshr9/shuttle/internal/netmon"
	"github.com/ggshr9/shuttle/mesh"
	"github.com/ggshr9/shuttle/provider"
	"github.com/ggshr9/shuttle/proxy"
	"github.com/ggshr9/shuttle/router"
	"github.com/ggshr9/shuttle/router/geodata"
	"github.com/ggshr9/shuttle/stream"
	"github.com/ggshr9/shuttle/subscription"
	"github.com/ggshr9/shuttle/transport/selector"

	"context"
	"fmt"
	"net"
)

// Compile-time check: Engine still satisfies plugin.ConnEmitter so existing
// code that passes *Engine to NewConnTracker continues to work.
var _ interface {
	EmitConnectionEvent(connID, state, target, rule, protocol, processName string, bytesIn, bytesOut, durationMs int64)
} = (*Engine)(nil)

// Compile-time check: MeshClient satisfies the MeshPacketHandler interface.
var _ proxy.MeshPacketHandler = (*mesh.MeshClient)(nil)

// Engine is the core shuttle client, managing transports, routing, and local proxies.
// It delegates observability (metrics, events, plugin chain) to ObservabilityManager
// and data-plane wiring (dialers, inbound/outbound setup) to TrafficManager.
type Engine struct {
	mu    sync.RWMutex
	state EngineState
	cfg   *config.ClientConfig

	logger  *slog.Logger
	obs     *ObservabilityManager
	traffic *TrafficManager

	// lifecycleMu serialises Start/Stop/Reload so that concurrent callers
	// cannot interleave their long-running init/shutdown sequences.
	lifecycleMu sync.Mutex

	sel       *selector.Selector
	cancel    context.CancelFunc
	parentCtx context.Context // stored for Reload

	// Closers for local proxy servers
	closers []func() error

	// Mesh manager — owns mesh connection lifecycle independently
	meshManager *MeshManager

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

	// Proactive migrator for network change detection
	proactiveMigrator *ProactiveMigrator

	// Background goroutine tracking for clean shutdown
	bgWg sync.WaitGroup

	// Connection sequence counter for generating correlation IDs
	connSeq uint64

	// Inbound/outbound abstraction layer
	inbounds  []adapter.Inbound
	outbounds map[string]adapter.Outbound

	// Subscription manager for auto-refreshing server lists
	subscriptionManager *subscription.Manager

	// Proxy and rule providers (ecosystem compat)
	proxyProviders []*provider.ProxyProvider
	ruleProviders  []*provider.RuleProvider

	// lastConfigErr records the most recent configuration validation error,
	// or nil when the active config validated cleanly. Used by the deep
	// readiness probe (gui/api/health_deep.go).
	lastConfigErr error

	// metrics holds engine-side counters/histograms surfaced via Metrics().
	// Allocated once in New() and never reset across Reload() — counters
	// are monotonically increasing for the lifetime of the engine.
	metrics *engineMetrics
}

// New creates a new Engine from the given config.
func New(cfg *config.ClientConfig) *Engine {
	logger := logutil.NewLogger(cfg.Log.Level, cfg.Log.Format)

	return &Engine{
		state:       StateStopped,
		cfg:         cfg,
		logger:      logger,
		obs:         NewObservabilityManager(logger),
		traffic:     NewTrafficManager(logger),
		meshManager: NewMeshManager(logger),
		metrics:     newEngineMetrics(),
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
	cfg := e.cfg
	st := e.streamTracker
	cb := e.circuitBreaker
	e.mu.RUnlock()

	// Read mesh client outside the main lock — MeshManager has its own mutex.
	var mc *mesh.MeshClient
	if e.meshManager != nil {
		mc = e.meshManager.Client()
	}

	stats := e.obs.Metrics().Stats()
	up, down := e.obs.Metrics().Speed()

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
		status.DrainingConns = sel.DrainingCount()
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

// StateName returns the human-readable engine state. Used by the deep
// readiness probe in gui/api/health_deep.go to satisfy the engineProbe
// interface.
func (e *Engine) StateName() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.state.String()
}

// HealthyOutbounds returns the count of outbounds currently usable. The
// engine populates the outbounds map at startInbounds time; while the
// engine is stopped the map is nil. We do not currently maintain a
// separate per-outbound liveness check at the engine level (each
// OutboundGroup with a url-test strategy runs its own healthcheck.Checker
// — see outbound_group_urltest.go), so "configured & built" is the best
// engine-wide approximation. Returns 0 when the engine is stopped.
func (e *Engine) HealthyOutbounds() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.outbounds)
}

// ConfigValid reports whether the active config most recently validated
// cleanly. False after a Start() / Reload() that failed validation, true
// once a subsequent successful validation has occurred.
func (e *Engine) ConfigValid() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastConfigErr == nil
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

// ProbeSnapshots returns the latest probe results from the active selector,
// translated into ProbeSnapshot values for use by OutboundGroup quality strategy.
// Returns nil if no selector is active.
func (e *Engine) ProbeSnapshots() map[string]ProbeSnapshot {
	sel := e.selector()
	if sel == nil {
		return nil
	}
	probes := sel.Probes()
	result := make(map[string]ProbeSnapshot, len(probes))
	for name, pr := range probes {
		result[name] = ProbeSnapshot{
			Latency:   pr.Latency,
			Loss:      pr.Loss,
			Available: pr.Available,
		}
	}
	return result
}

// SetTransportStrategy changes the active selector's strategy at runtime.
// Returns an error if no selector is active or the strategy string is unknown.
func (e *Engine) SetTransportStrategy(ctx context.Context, strategy string) error {
	sel := e.selector()
	if sel == nil {
		return fmt.Errorf("no active selector")
	}
	var s selector.Strategy
	switch strategy {
	case "auto":
		s = selector.StrategyAuto
	case "priority":
		s = selector.StrategyPriority
	case "latency":
		s = selector.StrategyLatency
	case "multipath":
		s = selector.StrategyMultipath
	default:
		return fmt.Errorf("unknown strategy: %q", strategy)
	}
	return sel.SetStrategy(ctx, s)
}

// ConnectMeshPeer initiates a P2P connection to the mesh peer identified by
// its virtual IP address string (e.g. "10.7.0.2"). Returns an error if mesh
// is not enabled, not connected, or the VIP is invalid.
func (e *Engine) ConnectMeshPeer(ctx context.Context, vip string) error {
	e.mu.RLock()
	mm := e.meshManager
	e.mu.RUnlock()

	if mm == nil {
		return fmt.Errorf("mesh is not enabled")
	}
	mc := mm.Client()
	if mc == nil {
		return fmt.Errorf("mesh is not connected")
	}
	ip := net.ParseIP(vip)
	if ip == nil {
		return fmt.Errorf("invalid virtual IP: %q", vip)
	}
	return mc.ConnectPeer(ctx, ip)
}
