package vmess

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

// Handler is called for authenticated VMess connections.
type Handler func(ctx context.Context, cmd byte, address string, conn net.Conn)

// ServerConfig holds VMess server configuration.
type ServerConfig struct {
	// Users maps UUID (16-byte array) to a user tag string.
	Users map[[16]byte]string `json:"users" yaml:"users"`
}

// Server accepts VMess connections, authenticates via AEAD-encrypted headers,
// and dispatches to a handler.
type Server struct {
	config ServerConfig
	logger *log.Logger

	// authCache maps recent auth_ids to UUIDs for O(1) lookup.
	// In a production implementation this would be time-bounded;
	// here we recompute per-connection for simplicity.
}

// NewServer creates a VMess server with the given config.
func NewServer(cfg ServerConfig, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.Default()
	}
	return &Server{config: cfg, logger: logger}
}

// Serve accepts connections from ln and handles them until ctx is cancelled.
func (s *Server) Serve(ctx context.Context, ln net.Listener, handler Handler) error {
	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return fmt.Errorf("vmess/server: accept: %w", err)
			}
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			s.handleConn(ctx, conn, handler)
		}()
	}
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn, handler Handler) {
	defer conn.Close()

	// Set a deadline for the auth header exchange.
	conn.SetReadDeadline(time.Now().Add(10 * time.Second)) //nolint:errcheck

	br := bufio.NewReader(conn)

	hdr, err := DecodeRequest(br, s.lookupAuthID)
	if err != nil {
		s.logger.Printf("vmess/server: decode request: %v", err)
		return
	}

	if hdr.Version != Version {
		s.logger.Printf("vmess/server: unsupported version 0x%02x", hdr.Version)
		conn.Write([]byte{ResponseError}) //nolint:errcheck
		return
	}

	// Clear deadline for data relay.
	conn.SetReadDeadline(time.Time{}) //nolint:errcheck

	// Send OK response.
	if _, err := conn.Write([]byte{ResponseOK}); err != nil {
		s.logger.Printf("vmess/server: write response: %v", err)
		return
	}

	// Wrap buffered reader with the original conn for writes.
	wrapped := &bufferedConn{Reader: br, Conn: conn}
	handler(ctx, hdr.Command, hdr.Address, wrapped)
}

// lookupAuthID tries all known UUIDs against the received auth_id.
// VMess AEAD auth_id = HMAC-MD5(uuid, timestamp). We try timestamps within
// a +/-120s window to account for clock skew.
func (s *Server) lookupAuthID(authID [AuthIDLen]byte) ([16]byte, bool) {
	now := time.Now().Unix()

	for uuid := range s.config.Users {
		// Check timestamps in a 240-second window (now +/- 120s).
		for delta := int64(-120); delta <= 120; delta++ {
			candidate := computeAuthID(uuid, now+delta)
			if authID == candidate {
				return uuid, true
			}
		}
	}

	return [16]byte{}, false
}

// authIDBytes converts an auth_id byte slice to a fixed array.
func authIDBytes(b []byte) [AuthIDLen]byte {
	var out [AuthIDLen]byte
	copy(out[:], b)
	return out
}

// readUint16 reads a big-endian uint16 from r.
func readUint16(r io.Reader) (uint16, error) {
	var buf [2]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(buf[:]), nil
}

// bufferedConn wraps a bufio.Reader (for reads) with the original net.Conn.
type bufferedConn struct {
	io.Reader
	net.Conn
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.Reader.Read(p)
}
