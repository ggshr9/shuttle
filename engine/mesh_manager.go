package engine

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/mesh"
	"github.com/shuttleX/shuttle/proxy"
	"github.com/shuttleX/shuttle/transport/selector"
)

// MeshManager owns the mesh VPN connection lifecycle independently from the
// Engine's core proxy path. It connects to the server via the transport
// selector, performs the mesh handshake, and injects the MeshClient into the
// TUN device for packet-level mesh routing.
type MeshManager struct {
	mu     sync.Mutex
	client *mesh.MeshClient
	logger *slog.Logger
	bgWg   sync.WaitGroup
}

// NewMeshManager creates a MeshManager with the given logger.
// If logger is nil, slog.Default() is used.
func NewMeshManager(logger *slog.Logger) *MeshManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &MeshManager{logger: logger}
}

// Client returns the current MeshClient, or nil if mesh is not connected.
func (mm *MeshManager) Client() *mesh.MeshClient {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	return mm.client
}

// Start connects to the mesh network with retry logic and wires the mesh
// client into the TUN device. It returns nil (no error) for non-fatal
// conditions like mesh being disabled or TUN not being available.
func (mm *MeshManager) Start(ctx context.Context, cfg *config.ClientConfig, sel *selector.Selector, tunInbound *proxy.TUNInbound) error {
	if !cfg.Mesh.Enabled {
		return nil
	}

	if tunInbound == nil || tunInbound.Server() == nil {
		mm.logger.Warn("mesh requires TUN to be enabled, skipping mesh")
		return nil
	}

	tunServer := tunInbound.Server()
	serverAddr := cfg.Server.Addr

	const maxRetries = 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		conn, err := sel.Dial(ctx, serverAddr)
		if err != nil {
			lastErr = fmt.Errorf("mesh: dial attempt %d: %w", attempt, err)
			mm.logger.Warn("mesh dial failed", "attempt", attempt, "err", err)
			if attempt < maxRetries {
				select {
				case <-ctx.Done():
					return fmt.Errorf("mesh: context cancelled during retry: %w", ctx.Err())
				case <-time.After(time.Duration(attempt) * time.Second):
				}
			}
			continue
		}

		openStream := func(ctx context.Context) (io.ReadWriteCloser, error) {
			return conn.OpenStream(ctx)
		}
		mc, err := mesh.NewMeshClient(ctx, openStream)
		if err != nil {
			lastErr = fmt.Errorf("mesh: handshake attempt %d: %w", attempt, err)
			mm.logger.Warn("mesh handshake failed", "attempt", attempt, "err", err)
			conn.Close()
			if attempt < maxRetries {
				select {
				case <-ctx.Done():
					return fmt.Errorf("mesh: context cancelled during retry: %w", ctx.Err())
				case <-time.After(time.Duration(attempt) * time.Second):
				}
			}
			continue
		}

		// Success — store client and wire into TUN.
		mm.mu.Lock()
		mm.client = mc
		mm.mu.Unlock()

		tunServer.MeshHandler = mc
		if err := tunServer.AddMeshRoute(mc.MeshCIDR()); err != nil {
			mm.logger.Warn("mesh: add route failed", "err", err)
		}

		mm.bgWg.Add(1)
		go func() {
			defer mm.bgWg.Done()
			tunServer.MeshReceiveLoop(ctx)
		}()

		mm.logger.Info("mesh connected",
			"virtualIP", mc.VirtualIP().String(),
			"cidr", mc.MeshCIDR(),
		)
		return nil
	}

	return fmt.Errorf("mesh: all %d attempts failed: %w", maxRetries, lastErr)
}

// Close shuts down the mesh client and waits for background goroutines.
func (mm *MeshManager) Close() error {
	mm.mu.Lock()
	mc := mm.client
	mm.client = nil
	mm.mu.Unlock()

	if mc != nil {
		mc.Close()
	}

	mm.bgWg.Wait()
	return nil
}
