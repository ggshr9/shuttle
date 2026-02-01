package proxy

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
)

const (
	socks5Version = 0x05
	// Auth methods
	authNone     = 0x00
	authPassword = 0x02
	authNoAccept = 0xFF
	// Commands
	cmdConnect = 0x01
	// Address types
	atypIPv4   = 0x01
	atypDomain = 0x03
	atypIPv6   = 0x04
	// Reply codes
	repSuccess          = 0x00
	repGeneralFailure   = 0x01
	repNotAllowed       = 0x02
	repNetworkUnreach   = 0x03
	repHostUnreach      = 0x04
	repConnRefused      = 0x05
	repCmdNotSupported  = 0x07
	repAddrNotSupported = 0x08
)

// contextKeyProcess is the context key for the source process name.
type contextKeyProcess struct{}

// WithProcess returns a context carrying the source process name.
func WithProcess(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, contextKeyProcess{}, name)
}

// ProcessFromContext extracts the source process name from context.
func ProcessFromContext(ctx context.Context) string {
	v, _ := ctx.Value(contextKeyProcess{}).(string)
	return v
}

// Dialer is a function that dials a target address, potentially through a proxy.
type Dialer func(ctx context.Context, network, addr string) (net.Conn, error)

// SOCKS5Config configures the SOCKS5 server.
type SOCKS5Config struct {
	ListenAddr string
	Username   string
	Password   string
}

// ProcResolver resolves a local TCP port to a process name.
type ProcResolver interface {
	Resolve(localPort uint16) string
}

// SOCKS5Server implements a SOCKS5 proxy server.
type SOCKS5Server struct {
	config       *SOCKS5Config
	dialer       Dialer
	listener     net.Listener
	closed       atomic.Bool
	wg           sync.WaitGroup
	logger       *slog.Logger
	ProcResolver ProcResolver
}

// NewSOCKS5Server creates a new SOCKS5 server.
func NewSOCKS5Server(cfg *SOCKS5Config, dialer Dialer, logger *slog.Logger) *SOCKS5Server {
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "127.0.0.1:1080"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &SOCKS5Server{
		config: cfg,
		dialer: dialer,
		logger: logger,
	}
}

// Start begins listening for SOCKS5 connections.
func (s *SOCKS5Server) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("socks5 listen: %w", err)
	}
	s.listener = ln
	s.logger.Info("socks5 server listening", "addr", s.config.ListenAddr)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.acceptLoop(ctx)
	}()
	return nil
}

func (s *SOCKS5Server) acceptLoop(ctx context.Context) {
	for {
		if s.closed.Load() {
			return
		}
		conn, err := s.listener.Accept()
		if err != nil {
			if s.closed.Load() {
				return
			}
			s.logger.Error("socks5 accept error", "err", err)
			continue
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(ctx, conn)
		}()
	}
}

func (s *SOCKS5Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	// 1. Auth negotiation
	if err := s.handleAuth(conn); err != nil {
		s.logger.Debug("socks5 auth failed", "err", err, "remote", conn.RemoteAddr())
		return
	}

	// 2. Read request
	target, err := s.handleRequest(conn)
	if err != nil {
		s.logger.Debug("socks5 request failed", "err", err)
		return
	}

	// 2.5 Resolve source process
	if s.ProcResolver != nil {
		if tcpAddr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
			if procName := s.ProcResolver.Resolve(uint16(tcpAddr.Port)); procName != "" {
				ctx = WithProcess(ctx, procName)
				s.logger.Debug("socks5 process identified", "process", procName, "target", target)
			}
		}
	}

	// 3. Connect to target
	s.logger.Debug("socks5 connect", "target", target)
	remote, err := s.dialer(ctx, "tcp", target)
	if err != nil {
		s.sendReply(conn, repHostUnreach, nil)
		s.logger.Debug("socks5 dial failed", "target", target, "err", err)
		return
	}
	defer remote.Close()

	// 4. Send success reply
	s.sendReply(conn, repSuccess, remote.LocalAddr())

	// 5. Relay data
	relay(conn, remote)
}

func (s *SOCKS5Server) handleAuth(conn net.Conn) error {
	// Read version + number of methods
	buf := make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return fmt.Errorf("read auth header: %w", err)
	}
	if buf[0] != socks5Version {
		return fmt.Errorf("unsupported version: %d", buf[0])
	}
	nMethods := int(buf[1])
	methods := make([]byte, nMethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return fmt.Errorf("read auth methods: %w", err)
	}

	// Select auth method
	if s.config.Username != "" {
		// Require password auth
		conn.Write([]byte{socks5Version, authPassword})
		return s.handlePasswordAuth(conn)
	}

	// No auth required
	_, err := conn.Write([]byte{socks5Version, authNone})
	return err
}

func (s *SOCKS5Server) handlePasswordAuth(conn net.Conn) error {
	// RFC 1929: username/password auth
	buf := make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return err
	}
	// buf[0] = version (0x01), buf[1] = username length
	ulen := int(buf[1])
	username := make([]byte, ulen)
	if _, err := io.ReadFull(conn, username); err != nil {
		return err
	}
	plenBuf := make([]byte, 1)
	if _, err := io.ReadFull(conn, plenBuf); err != nil {
		return err
	}
	password := make([]byte, int(plenBuf[0]))
	if _, err := io.ReadFull(conn, password); err != nil {
		return err
	}

	if string(username) == s.config.Username && string(password) == s.config.Password {
		conn.Write([]byte{0x01, 0x00}) // success
		return nil
	}
	conn.Write([]byte{0x01, 0x01}) // failure
	return fmt.Errorf("auth failed")
}

func (s *SOCKS5Server) handleRequest(conn net.Conn) (string, error) {
	// Read: VER CMD RSV ATYP
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return "", fmt.Errorf("read request: %w", err)
	}
	if buf[0] != socks5Version {
		return "", fmt.Errorf("unsupported version: %d", buf[0])
	}
	if buf[1] != cmdConnect {
		s.sendReply(conn, repCmdNotSupported, nil)
		return "", fmt.Errorf("unsupported command: %d", buf[1])
	}

	// Read address
	var host string
	switch buf[3] {
	case atypIPv4:
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", err
		}
		host = net.IP(addr).String()
	case atypDomain:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return "", err
		}
		domain := make([]byte, int(lenBuf[0]))
		if _, err := io.ReadFull(conn, domain); err != nil {
			return "", err
		}
		host = string(domain)
	case atypIPv6:
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", err
		}
		host = net.IP(addr).String()
	default:
		s.sendReply(conn, repAddrNotSupported, nil)
		return "", fmt.Errorf("unsupported address type: %d", buf[3])
	}

	// Read port
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", err
	}
	port := binary.BigEndian.Uint16(portBuf)

	return fmt.Sprintf("%s:%d", host, port), nil
}

func (s *SOCKS5Server) sendReply(conn net.Conn, rep byte, addr net.Addr) {
	reply := []byte{socks5Version, rep, 0x00, atypIPv4, 0, 0, 0, 0, 0, 0}
	if addr != nil {
		if tcpAddr, ok := addr.(*net.TCPAddr); ok {
			ip := tcpAddr.IP.To4()
			if ip != nil {
				copy(reply[4:8], ip)
				binary.BigEndian.PutUint16(reply[8:10], uint16(tcpAddr.Port))
			}
		}
	}
	conn.Write(reply)
}

// relay copies data bidirectionally between two connections.
func relay(a, b net.Conn) {
	done := make(chan struct{}, 2)
	go func() {
		io.Copy(b, a)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(a, b)
		done <- struct{}{}
	}()
	<-done
	<-done
}

// Close shuts down the SOCKS5 server.
func (s *SOCKS5Server) Close() error {
	s.closed.Store(true)
	if s.listener != nil {
		s.listener.Close()
	}
	s.wg.Wait()
	return nil
}
