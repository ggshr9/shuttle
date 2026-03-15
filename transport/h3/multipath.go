package h3

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shuttle-proxy/shuttle/transport"
)

// MultipathConfig configures multipath behavior.
type MultipathConfig struct {
	Enabled       bool          `yaml:"enabled" json:"enabled"`
	Interfaces    []string      `yaml:"interfaces,omitempty" json:"interfaces,omitempty"` // bind to specific interfaces, empty = auto-detect
	Mode          string        `yaml:"mode" json:"mode"`                                 // "redundant" | "aggregate" | "failover"
	ProbeInterval time.Duration `yaml:"probe_interval" json:"probe_interval"`
}

// MultipathManager manages multiple network paths to the same server.
type MultipathManager struct {
	mu          sync.RWMutex
	paths       []*networkPath
	activePath  int // index of primary path
	mode        string
	logger      *slog.Logger
	probeCancel context.CancelFunc
	rrIndex     atomic.Uint64 // round-robin counter for aggregate mode
}

type networkPath struct {
	iface     string
	localAddr net.Addr
	conn      transport.Connection
	rtt       atomic.Int64 // nanoseconds
	loss      atomic.Int64 // loss rate * 1000 (permille)
	bytesSent atomic.Int64
	bytesRecv atomic.Int64
	available atomic.Bool
	lastProbe atomic.Int64 // unix nano
}

// PathStats holds per-path statistics returned by Stats().
type PathStats struct {
	Interface string  `json:"interface"`
	LocalAddr string  `json:"local_addr"`
	RTT       int64   `json:"rtt_ms"`
	LossRate  float64 `json:"loss_rate"`
	BytesSent int64   `json:"bytes_sent"`
	BytesRecv int64   `json:"bytes_recv"`
	Available bool    `json:"available"`
}

// NewMultipathManager creates a new MultipathManager with the given config.
func NewMultipathManager(cfg *MultipathConfig, logger *slog.Logger) *MultipathManager {
	if logger == nil {
		logger = slog.Default()
	}
	mode := cfg.Mode
	if mode == "" {
		mode = "failover"
	}
	return &MultipathManager{
		mode:   mode,
		logger: logger,
	}
}

// DiscoverPaths finds available network interfaces and returns their names.
// It filters to interfaces that are up and have at least one unicast address,
// and optionally restricts to the configured interface names.
func (m *MultipathManager) DiscoverPaths() ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list interfaces: %w", err)
	}

	var result []string
	for _, iface := range ifaces {
		// Skip loopback and down interfaces.
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil || len(addrs) == 0 {
			continue
		}

		result = append(result, iface.Name)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no usable network interfaces found")
	}

	m.logger.Info("discovered network interfaces", "count", len(result), "interfaces", result)
	return result, nil
}

// AddPath adds a specific network path with the given interface name and connection.
func (m *MultipathManager) AddPath(iface string, conn transport.Connection) {
	p := &networkPath{
		iface:     iface,
		localAddr: conn.LocalAddr(),
		conn:      conn,
	}
	p.available.Store(true)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.paths = append(m.paths, p)
	m.logger.Info("added network path", "interface", iface, "local_addr", conn.LocalAddr())
}

// SelectPath returns the best path based on current mode.
//   - "failover": use primary, switch on failure
//   - "redundant": use the path with lowest RTT among available paths
//   - "aggregate": round-robin across healthy paths
func (m *MultipathManager) SelectPath() *networkPath {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.paths) == 0 {
		return nil
	}

	switch m.mode {
	case "failover":
		return m.selectFailover()
	case "aggregate":
		return m.selectAggregate()
	case "redundant":
		return m.selectRedundant()
	default:
		return m.selectFailover()
	}
}

func (m *MultipathManager) selectFailover() *networkPath {
	// Try active path first.
	if m.activePath < len(m.paths) && m.paths[m.activePath].available.Load() {
		return m.paths[m.activePath]
	}
	// Find first available path.
	for i, p := range m.paths {
		if p.available.Load() {
			m.activePath = i
			return p
		}
	}
	return nil
}

func (m *MultipathManager) selectAggregate() *networkPath {
	// Collect available paths.
	available := make([]*networkPath, 0, len(m.paths))
	for _, p := range m.paths {
		if p.available.Load() {
			available = append(available, p)
		}
	}
	if len(available) == 0 {
		return nil
	}
	idx := m.rrIndex.Add(1) - 1
	return available[idx%uint64(len(available))]
}

func (m *MultipathManager) selectRedundant() *networkPath {
	// Select path with lowest RTT among available paths.
	var best *networkPath
	var bestRTT int64 = -1
	for _, p := range m.paths {
		if !p.available.Load() {
			continue
		}
		rtt := p.rtt.Load()
		if bestRTT < 0 || rtt < bestRTT {
			best = p
			bestRTT = rtt
		}
	}
	return best
}

// StartProbes begins periodic path quality monitoring.
func (m *MultipathManager) StartProbes(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	m.probeCancel = cancel
	m.mu.Unlock()

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.probe()
			}
		}
	}()
}

func (m *MultipathManager) probe() {
	m.mu.RLock()
	paths := make([]*networkPath, len(m.paths))
	copy(paths, m.paths)
	m.mu.RUnlock()

	for _, p := range paths {
		now := time.Now().UnixNano()
		p.lastProbe.Store(now)

		// Probe by attempting to open and immediately close a stream.
		// This checks if the underlying QUIC connection is still alive.
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		stream, err := p.conn.OpenStream(ctx)
		cancel()
		if err != nil {
			p.available.Store(false)
			m.logger.Debug("path probe failed", "interface", p.iface, "error", err)
			continue
		}
		stream.Close()
		rtt := time.Since(start).Nanoseconds()
		p.rtt.Store(rtt)
		p.available.Store(true)
	}
}

// Stats returns per-path statistics.
func (m *MultipathManager) Stats() []PathStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make([]PathStats, len(m.paths))
	for i, p := range m.paths {
		localAddr := ""
		if p.localAddr != nil {
			localAddr = p.localAddr.String()
		}
		stats[i] = PathStats{
			Interface: p.iface,
			LocalAddr: localAddr,
			RTT:       p.rtt.Load() / int64(time.Millisecond),
			LossRate:  float64(p.loss.Load()) / 1000.0,
			BytesSent: p.bytesSent.Load(),
			BytesRecv: p.bytesRecv.Load(),
			Available: p.available.Load(),
		}
	}
	return stats
}

// Close stops probes and closes all paths.
func (m *MultipathManager) Close() {
	m.mu.Lock()
	if m.probeCancel != nil {
		m.probeCancel()
		m.probeCancel = nil
	}
	paths := m.paths
	m.paths = nil
	m.mu.Unlock()

	for _, p := range paths {
		if p.conn != nil {
			p.conn.Close()
		}
	}
}

// multipathConn wraps multiple connections with path selection.
type multipathConn struct {
	manager *MultipathManager
	primary transport.Connection
}

func (c *multipathConn) OpenStream(ctx context.Context) (transport.Stream, error) {
	path := c.manager.SelectPath()
	if path == nil {
		// Fall back to primary.
		return c.primary.OpenStream(ctx)
	}
	s, err := path.conn.OpenStream(ctx)
	if err != nil {
		// On failure, mark path unavailable and try primary.
		path.available.Store(false)
		if path.conn != c.primary {
			return c.primary.OpenStream(ctx)
		}
		return nil, err
	}
	return &multipathStream{Stream: s, path: path}, nil
}

func (c *multipathConn) AcceptStream(ctx context.Context) (transport.Stream, error) {
	return c.primary.AcceptStream(ctx)
}

func (c *multipathConn) Close() error {
	c.manager.Close()
	return nil
}

func (c *multipathConn) LocalAddr() net.Addr  { return c.primary.LocalAddr() }
func (c *multipathConn) RemoteAddr() net.Addr { return c.primary.RemoteAddr() }

// multipathStream wraps a stream and tracks bytes on its path.
type multipathStream struct {
	transport.Stream
	path *networkPath
}

func (s *multipathStream) Read(p []byte) (int, error) {
	n, err := s.Stream.Read(p)
	if n > 0 {
		s.path.bytesRecv.Add(int64(n))
	}
	return n, err
}

func (s *multipathStream) Write(p []byte) (int, error) {
	n, err := s.Stream.Write(p)
	if n > 0 {
		s.path.bytesSent.Add(int64(n))
	}
	return n, err
}

var _ transport.Connection = (*multipathConn)(nil)
