package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/ggshr9/shuttle/adapter"
	"github.com/ggshr9/shuttle/config"
	"github.com/ggshr9/shuttle/internal/dnsclass"
	"github.com/ggshr9/shuttle/internal/relay"
)

// inboundListener holds the state for one per-request protocol inbound.
type inboundListener struct {
	tag      string
	handler  adapter.InboundHandler
	listener net.Listener
}

// startInbounds creates and starts per-request protocol inbound handlers
// (Shadowsocks, VLESS, Trojan, etc.) from ServerConfig.Inbounds.
// Returns a slice of inboundListeners that must be closed on shutdown.
func (s *Server) startInbounds(ctx context.Context) ([]*inboundListener, error) {
	listeners := make([]*inboundListener, 0, len(s.cfg.Inbounds))

	for _, inCfg := range s.cfg.Inbounds {
		il, err := s.startOneInbound(ctx, inCfg)
		if err != nil {
			// Clean up already-started listeners on failure.
			for _, l := range listeners {
				l.listener.Close()
				l.handler.Close()
			}
			return nil, fmt.Errorf("inbound %q (%s): %w", inCfg.Tag, inCfg.Type, err)
		}
		listeners = append(listeners, il)
	}

	return listeners, nil
}

func (s *Server) startOneInbound(ctx context.Context, inCfg config.InboundConfig) (*inboundListener, error) {
	// Look up the DialerFactory (which also provides NewInboundHandler).
	df := adapter.GetDialerFactory(inCfg.Type)
	if df == nil {
		return nil, fmt.Errorf("no dialer factory for type %q", inCfg.Type)
	}

	// Convert json.RawMessage options to map[string]any.
	opts := make(map[string]any)
	if len(inCfg.Options) > 0 {
		if err := json.Unmarshal(inCfg.Options, &opts); err != nil {
			return nil, fmt.Errorf("unmarshal options: %w", err)
		}
	}

	handler, err := df.NewInboundHandler(opts, adapter.FactoryOptions{Logger: s.logger})
	if err != nil {
		return nil, fmt.Errorf("create handler: %w", err)
	}

	// Determine listen address.
	listenAddr := inCfg.Listen
	if listenAddr == "" {
		if v, ok := opts["listen"].(string); ok {
			listenAddr = v
		}
	}
	if listenAddr == "" {
		handler.Close()
		return nil, fmt.Errorf("no listen address configured")
	}

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		handler.Close()
		return nil, fmt.Errorf("listen %s: %w", listenAddr, err)
	}

	il := &inboundListener{
		tag:      inCfg.Tag,
		handler:  handler,
		listener: ln,
	}

	s.logger.Info("inbound handler started",
		"tag", inCfg.Tag,
		"type", inCfg.Type,
		"listen", listenAddr,
	)

	// Start serving in a goroutine. The ConnHandler relays each connection
	// to its destination, reusing the same relay logic as the main handler.
	go func() {
		if err := handler.Serve(ctx, ln, s.inboundConnHandler(ctx)); err != nil {
			if ctx.Err() == nil {
				s.logger.Error("inbound handler stopped with error",
					"tag", inCfg.Tag, "type", inCfg.Type, "err", err)
			}
		}
	}()

	return il, nil
}

// inboundConnHandler returns an adapter.ConnHandler that relays connections
// to their destinations. This bridges per-request protocol inbounds into
// the same relay pipeline used by the main transport handler.
func (s *Server) inboundConnHandler(ctx context.Context) adapter.ConnHandler {
	return func(_ context.Context, conn net.Conn, meta adapter.ConnMetadata) {
		defer conn.Close()

		target := meta.Destination
		if target == "" {
			s.logger.Debug("inbound: empty destination, dropping")
			return
		}

		// SSRF protection
		if !s.handler.AllowPrivateNetworks && IsBlockedTarget(target) {
			s.logger.Warn("inbound: blocked SSRF attempt", "target", target)
			return
		}

		s.logger.Debug("inbound: proxying", "target", target, "network", meta.Network)

		// Metrics tracking
		transportName := "inbound-" + meta.Protocol
		if meta.InboundTag != "" {
			transportName = "inbound-" + meta.InboundTag
		}
		s.metrics.ConnOpened(transportName)
		connStart := time.Now()
		defer func() { s.metrics.ConnClosed(transportName, time.Since(connStart)) }()

		if s.adminInfo != nil {
			s.adminInfo.TotalConns.Add(1)
			s.adminInfo.ActiveConns.Add(1)
			defer s.adminInfo.ActiveConns.Add(-1)
		}

		// Dial destination
		remote, err := net.DialTimeout("tcp", target, 10*time.Second)
		if err != nil {
			s.logger.Debug("inbound: dial target failed", "target", target, "err", err)
			if s.metrics != nil {
				s.metrics.RecordDestResolveFailure(dnsclass.Classify(err))
			}
			return
		}
		defer remote.Close()

		// Run through plugin chain if available
		var trackedRemote net.Conn = remote
		if s.pluginChain != nil {
			wrapped, chainErr := s.pluginChain.OnConnect(remote, target)
			if chainErr != nil {
				s.logger.Debug("inbound: plugin chain rejected", "target", target, "err", chainErr)
				return
			}
			trackedRemote = wrapped
			defer s.pluginChain.OnDisconnect(wrapped)
		}

		// Bidirectional relay
		inboundRelay(conn, trackedRemote)
	}
}

// inboundRelay copies data bidirectionally between two connections.
func inboundRelay(a, b io.ReadWriteCloser) {
	_, _, _ = relay.Relay(a, b)
}
