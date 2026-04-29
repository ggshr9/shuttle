package trojan

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/ggshr9/shuttle/transport/shared"
)

// Handler is called for authenticated Trojan connections.
// cmd is shared.CmdConnect or shared.CmdUDPAssociate.
// address is the target "host:port".
// conn carries the remaining data after the Trojan header.
type Handler func(ctx context.Context, cmd byte, address string, conn net.Conn)

// ServerConfig holds Trojan server configuration.
type ServerConfig struct {
	// Passwords maps SHA224 hex hashes to user tags.
	Passwords map[string]string `json:"passwords" yaml:"passwords"`
	// Fallback is the address "host:port" of a cover site for unauthenticated connections.
	Fallback string `json:"fallback" yaml:"fallback"`
}

// Server accepts Trojan connections, authenticates them, and dispatches
// to a handler or falls back to a cover site.
type Server struct {
	config ServerConfig
	logger *log.Logger
}

// NewServer creates a Trojan server with the given config.
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
				return fmt.Errorf("trojan/server: accept: %w", err)
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

	// Read the first 58 bytes (56 hex + CRLF) with a deadline so non-Trojan
	// clients that send fewer bytes don't block forever.
	// We use a raw read+buffer instead of bufio.Peek to avoid caching
	// timeout errors in the bufio.Reader (which would break fallback relay).
	headerBuf := make([]byte, HashLen+2)
	conn.SetReadDeadline(time.Now().Add(1 * time.Second)) //nolint:errcheck
	n, err := io.ReadFull(conn, headerBuf)
	conn.SetReadDeadline(time.Time{}) //nolint:errcheck

	if err != nil {
		// Could not read a full header — relay whatever we got to fallback.
		prefix := headerBuf[:n]
		s.fallbackOrCloseRaw(prefix, conn)
		return
	}

	hash := string(headerBuf[:HashLen])
	crlfBytes := string(headerBuf[HashLen:])

	// Check authentication
	if crlfBytes != crlf {
		s.fallbackOrCloseRaw(headerBuf, conn)
		return
	}

	if _, ok := s.config.Passwords[hash]; !ok {
		s.fallbackOrCloseRaw(headerBuf, conn)
		return
	}

	// Authenticated — read the rest of the request (cmd + addr + CRLF).
	// Build a reader from the remaining conn data (header already consumed).
	br := bufio.NewReader(conn)

	// Read 1-byte command
	cmdBuf := [1]byte{}
	if _, err := io.ReadFull(br, cmdBuf[:]); err != nil {
		s.logger.Printf("trojan/server: read cmd: %v", err)
		return
	}
	cmd := cmdBuf[0]

	// Read SOCKS5-style address
	_, address, err := shared.DecodeAddr(br)
	if err != nil {
		s.logger.Printf("trojan/server: read addr: %v", err)
		return
	}

	// Read trailing CRLF
	crlfBuf := [2]byte{}
	if _, err := io.ReadFull(br, crlfBuf[:]); err != nil {
		s.logger.Printf("trojan/server: read trailing CRLF: %v", err)
		return
	}

	// Wrap the buffered reader with the underlying conn for writes.
	wrapped := &bufferedConn{Reader: br, Conn: conn}
	handler(ctx, cmd, address, wrapped)
}

// fallbackOrCloseRaw relays the connection to the fallback address (cover site),
// prepending any already-read prefix bytes. Closes if no fallback is configured.
func (s *Server) fallbackOrCloseRaw(prefix []byte, conn net.Conn) {
	if s.config.Fallback == "" {
		return
	}

	backend, err := net.Dial("tcp", s.config.Fallback)
	if err != nil {
		s.logger.Printf("trojan/server: fallback dial %s: %v", s.config.Fallback, err)
		return
	}
	defer backend.Close()

	// Build a reader that replays the prefix then continues from conn.
	var src io.Reader = conn
	if len(prefix) > 0 {
		src = io.MultiReader(bytes.NewReader(prefix), conn)
	}

	// Relay bidirectionally.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		io.Copy(backend, src) //nolint:errcheck
	}()
	go func() {
		defer wg.Done()
		io.Copy(conn, backend) //nolint:errcheck
	}()
	wg.Wait()
}

// bufferedConn wraps a bufio.Reader (for reads) with the original net.Conn (for writes and other methods).
type bufferedConn struct {
	io.Reader
	net.Conn
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.Reader.Read(p)
}
