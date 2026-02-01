package selector

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/shuttle-proxy/shuttle/transport"
)

// Strategy defines how transport selection works.
type Strategy string

const (
	StrategyAuto     Strategy = "auto"     // Automatically select best transport
	StrategyPriority Strategy = "priority" // Use first available in priority order
	StrategyLatency  Strategy = "latency"  // Use lowest latency transport
)

// Selector manages multiple transports and selects the best one.
type Selector struct {
	transports []transport.ClientTransport
	active     transport.ClientTransport
	activeConn transport.Connection
	strategy   Strategy
	probes     map[string]*ProbeResult
	mu         sync.RWMutex
	logger     *slog.Logger
}

// ProbeResult stores health check results for a transport.
type ProbeResult struct {
	Transport transport.ClientTransport
	Latency   time.Duration
	Loss      float64
	Available bool
	LastCheck time.Time
}

// Config configures the transport selector.
type Config struct {
	Strategy      Strategy
	ProbeInterval time.Duration
}

// New creates a new transport selector.
func New(transports []transport.ClientTransport, cfg *Config, logger *slog.Logger) *Selector {
	if cfg == nil {
		cfg = &Config{
			Strategy:      StrategyAuto,
			ProbeInterval: 30 * time.Second,
		}
	}
	if logger == nil {
		logger = slog.Default()
	}
	s := &Selector{
		transports: transports,
		strategy:   cfg.Strategy,
		probes:     make(map[string]*ProbeResult),
		logger:     logger,
	}
	for _, t := range transports {
		s.probes[t.Type()] = &ProbeResult{
			Transport: t,
			Available: true,
		}
	}
	return s
}

// Start begins periodic probing of all transports.
func (s *Selector) Start(ctx context.Context) {
	go s.probeLoop(ctx)
}

func (s *Selector) probeLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.probeAll(ctx)
		}
	}
}

func (s *Selector) probeAll(ctx context.Context) {
	for _, t := range s.transports {
		result := Probe(ctx, t)
		s.mu.Lock()
		s.probes[t.Type()] = result
		s.mu.Unlock()
	}
	s.maybeSwitch()
}

func (s *Selector) maybeSwitch() {
	s.mu.Lock()
	defer s.mu.Unlock()

	best := s.selectBest()
	if best == nil {
		return
	}
	if s.active != nil && s.active.Type() == best.Type() {
		return
	}
	s.logger.Info("switching transport", "from", s.activeType(), "to", best.Type())
	s.active = best
}

func (s *Selector) activeType() string {
	if s.active == nil {
		return "none"
	}
	return s.active.Type()
}

func (s *Selector) selectBest() transport.ClientTransport {
	switch s.strategy {
	case StrategyLatency:
		return s.lowestLatency()
	case StrategyPriority:
		return s.firstAvailable()
	default: // auto
		return s.autoSelect()
	}
}

func (s *Selector) lowestLatency() transport.ClientTransport {
	var best *ProbeResult
	for _, p := range s.probes {
		if !p.Available {
			continue
		}
		if best == nil || p.Latency < best.Latency {
			best = p
		}
	}
	if best != nil {
		return best.Transport
	}
	return nil
}

func (s *Selector) firstAvailable() transport.ClientTransport {
	for _, t := range s.transports {
		if p := s.probes[t.Type()]; p != nil && p.Available {
			return t
		}
	}
	return nil
}

func (s *Selector) autoSelect() transport.ClientTransport {
	// Prefer H3 > Reality > CDN, but switch if current is unavailable
	for _, t := range s.transports {
		if p := s.probes[t.Type()]; p != nil && p.Available {
			return t
		}
	}
	return nil
}

// Dial connects using the currently selected transport.
func (s *Selector) Dial(ctx context.Context, addr string) (transport.Connection, error) {
	s.mu.RLock()
	active := s.active
	s.mu.RUnlock()

	if active == nil {
		// Try each transport in order
		for _, t := range s.transports {
			conn, err := t.Dial(ctx, addr)
			if err == nil {
				s.mu.Lock()
				s.active = t
				s.activeConn = conn
				s.mu.Unlock()
				return conn, nil
			}
			s.logger.Debug("transport dial failed", "type", t.Type(), "err", err)
		}
		return nil, fmt.Errorf("all transports failed")
	}

	conn, err := active.Dial(ctx, addr)
	if err != nil {
		// Active transport failed, try fallback
		s.logger.Warn("active transport failed, trying fallback", "type", active.Type(), "err", err)
		return s.dialFallback(ctx, addr, active)
	}
	return conn, nil
}

func (s *Selector) dialFallback(ctx context.Context, addr string, failed transport.ClientTransport) (transport.Connection, error) {
	for _, t := range s.transports {
		if t.Type() == failed.Type() {
			continue
		}
		conn, err := t.Dial(ctx, addr)
		if err == nil {
			s.mu.Lock()
			s.active = t
			s.activeConn = conn
			s.mu.Unlock()
			s.logger.Info("fell back to transport", "type", t.Type())
			return conn, nil
		}
	}
	return nil, fmt.Errorf("all fallback transports failed")
}

func (s *Selector) Type() string { return "selector" }

func (s *Selector) Close() error {
	for _, t := range s.transports {
		t.Close()
	}
	return nil
}

var _ transport.ClientTransport = (*Selector)(nil)
