package transport

import (
	"context"
	"fmt"
	"net"

	"github.com/shuttleX/shuttle/adapter"
)

// ByteStreamServerConfig configures the server-side pipeline.
type ByteStreamServerConfig struct {
	Security []adapter.SecureWrapper // Security chain applied in order. May be nil.
	Auth     adapter.Authenticator   // Authentication after security. May be nil.
	Mux      adapter.Multiplexer     // Stream multiplexer (required).
}

// ByteStreamServerProcess takes an accepted raw connection and runs it through
// the server-side pipeline: security → auth → mux.
// Returns the multiplexed Connection and the authenticated user identity (may be empty).
func ByteStreamServerProcess(ctx context.Context, raw net.Conn, cfg ByteStreamServerConfig) (adapter.Connection, string, error) {
	success := false
	defer func() {
		if !success {
			raw.Close()
		}
	}()

	conn := raw
	var err error
	for _, wrapper := range cfg.Security {
		conn, err = wrapper.WrapServer(ctx, conn)
		if err != nil {
			return nil, "", fmt.Errorf("server security: %w", err)
		}
	}

	var user string
	if cfg.Auth != nil {
		user, err = cfg.Auth.AuthServer(conn)
		if err != nil {
			return nil, "", fmt.Errorf("server auth: %w", err)
		}
	}

	muxConn, err := cfg.Mux.Server(conn)
	if err != nil {
		return nil, "", fmt.Errorf("server mux: %w", err)
	}

	success = true
	return muxConn, user, nil
}
