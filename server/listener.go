package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"

	"github.com/shuttle-proxy/shuttle/transport"
)

type ListenerConfig struct {
	ListenAddr string // e.g., ":443"
}

// MultiListener manages multiple transport protocols on a shared port.
// H3 (QUIC) uses UDP and Reality (TLS) uses TCP, so they naturally
// coexist on the same port number without conflict.
type MultiListener struct {
	config     *ListenerConfig
	transports []transport.ServerTransport
	connCh     chan transport.Connection
	closed     atomic.Bool
	wg         sync.WaitGroup
	logger     *slog.Logger
}

func NewMultiListener(cfg *ListenerConfig, logger *slog.Logger) *MultiListener {
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":443"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &MultiListener{
		config: cfg,
		connCh: make(chan transport.Connection, 128),
		logger: logger,
	}
}

func (ml *MultiListener) AddTransport(t transport.ServerTransport) {
	ml.transports = append(ml.transports, t)
}

// Start begins listening on all registered transports.
// H3 binds UDP, Reality binds TCP — both can share the same port number.
func (ml *MultiListener) Start(ctx context.Context) error {
	if len(ml.transports) == 0 {
		return fmt.Errorf("no transports registered")
	}

	// Verify no duplicate protocol types
	seen := make(map[string]bool)
	for _, t := range ml.transports {
		if seen[t.Type()] {
			return fmt.Errorf("duplicate transport type: %s", t.Type())
		}
		seen[t.Type()] = true
	}

	for _, t := range ml.transports {
		if err := t.Listen(ctx); err != nil {
			// Clean up already-started transports
			for _, started := range ml.transports {
				if started == t {
					break
				}
				started.Close()
			}
			return fmt.Errorf("start transport %s: %w", t.Type(), err)
		}
		ml.logger.Info("transport started",
			"type", t.Type(),
			"addr", ml.config.ListenAddr,
			"proto", protoForType(t.Type()))

		ml.wg.Add(1)
		go func(tr transport.ServerTransport) {
			defer ml.wg.Done()
			ml.acceptLoop(ctx, tr)
		}(t)
	}

	ml.logger.Info("all transports started",
		"addr", ml.config.ListenAddr,
		"count", len(ml.transports))
	return nil
}

func protoForType(t string) string {
	switch t {
	case "h3":
		return "UDP (QUIC)"
	case "reality":
		return "TCP (TLS)"
	case "cdn":
		return "TCP (HTTP/2)"
	case "webrtc":
		return "UDP/TCP (WebRTC)"
	default:
		return "unknown"
	}
}

func (ml *MultiListener) acceptLoop(ctx context.Context, t transport.ServerTransport) {
	for {
		if ml.closed.Load() {
			return
		}
		conn, err := t.Accept(ctx)
		if err != nil {
			if ml.closed.Load() || ctx.Err() != nil {
				return
			}
			ml.logger.Error("accept error", "transport", t.Type(), "err", err)
			continue
		}
		ml.logger.Debug("new connection",
			"transport", t.Type(),
			"remote", conn.RemoteAddr())
		select {
		case ml.connCh <- conn:
		case <-ctx.Done():
			conn.Close()
			return
		}
	}
}

func (ml *MultiListener) Accept(ctx context.Context) (transport.Connection, error) {
	select {
	case conn := <-ml.connCh:
		return conn, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (ml *MultiListener) Close() error {
	ml.closed.Store(true)
	for _, t := range ml.transports {
		t.Close()
	}
	ml.wg.Wait()
	close(ml.connCh)
	return nil
}

func (ml *MultiListener) Addr() net.Addr {
	return &net.TCPAddr{}
}
