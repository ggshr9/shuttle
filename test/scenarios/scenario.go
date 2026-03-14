// Package scenarios provides integration tests that exercise multiple
// components together using the vnet simulator and fault injection framework.
// Each test represents a real user story (transport fallback, reconnection,
// congestion adaptation) and verifies end-to-end behavior without touching
// real networks.
package scenarios

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/yamux"
	"github.com/shuttle-proxy/shuttle/testkit/fault"
	"github.com/shuttle-proxy/shuttle/testkit/observe"
	"github.com/shuttle-proxy/shuttle/testkit/vnet"
	"github.com/shuttle-proxy/shuttle/transport"
)

// ---------------------------------------------------------------------------
// Env — lightweight test scenario environment
// ---------------------------------------------------------------------------

// Env holds a test scenario environment with virtual network + fault injection.
type Env struct {
	Net      *vnet.Network
	Faults   *fault.Injector
	Recorder *observe.Recorder
	T        testing.TB
}

// NewEnv creates a new scenario environment with optional vnet options.
// It automatically creates an observe.Recorder that dumps timeline on test failure.
func NewEnv(t testing.TB, opts ...vnet.Option) *Env {
	t.Helper()
	rec := observe.NewRecorder(t)
	opts = append(opts, vnet.WithRecorder(rec))
	return &Env{
		Net:      vnet.New(opts...),
		Faults:   fault.New().WithRecorder(rec),
		Recorder: rec,
		T:        t,
	}
}

// Close tears down the virtual network.
func (e *Env) Close() {
	e.Net.Close()
}

// ---------------------------------------------------------------------------
// yamux-over-vnet transport — adapts the conformance pipe_test.go pattern
// to work over vnet connections instead of net.Pipe().
// ---------------------------------------------------------------------------

// vnetStream wraps a yamux.Stream to satisfy transport.Stream.
type vnetStream struct {
	ys *yamux.Stream
	id uint64
}

func (s *vnetStream) Read(p []byte) (int, error)  { return s.ys.Read(p) }
func (s *vnetStream) Write(p []byte) (int, error) { return s.ys.Write(p) }
func (s *vnetStream) Close() error                { return s.ys.Close() }
func (s *vnetStream) StreamID() uint64            { return s.id }

var _ transport.Stream = (*vnetStream)(nil)

// vnetConn wraps a yamux.Session to satisfy transport.Connection.
type vnetConn struct {
	sess      *yamux.Session
	local     net.Addr
	remote    net.Addr
	streamSeq atomic.Uint64
}

func (c *vnetConn) OpenStream(ctx context.Context) (transport.Stream, error) {
	ys, err := c.sess.OpenStream()
	if err != nil {
		return nil, err
	}
	id := c.streamSeq.Add(1)
	return &vnetStream{ys: ys, id: id}, nil
}

func (c *vnetConn) AcceptStream(ctx context.Context) (transport.Stream, error) {
	type result struct {
		s   *yamux.Stream
		err error
	}
	ch := make(chan result, 1)
	go func() {
		s, err := c.sess.AcceptStream()
		ch <- result{s, err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return nil, r.err
		}
		id := c.streamSeq.Add(1)
		return &vnetStream{ys: r.s, id: id}, nil
	}
}

func (c *vnetConn) Close() error      { return c.sess.Close() }
func (c *vnetConn) LocalAddr() net.Addr  { return c.local }
func (c *vnetConn) RemoteAddr() net.Addr { return c.remote }

var _ transport.Connection = (*vnetConn)(nil)

// vnetAddr implements net.Addr for vnet-based transports.
type vnetAddr string

func (a vnetAddr) Network() string { return "vnet" }
func (a vnetAddr) String() string  { return string(a) }

// ---------------------------------------------------------------------------
// vnetClientTransport — ClientTransport over vnet
// ---------------------------------------------------------------------------

// vnetClientTransport dials through a vnet.Network from a specific node.
type vnetClientTransport struct {
	net      *vnet.Network
	node     *vnet.Node
	typeName string
	dialFn   func(ctx context.Context, addr string) (net.Conn, error) // optional override
	mu       sync.Mutex
	closed   bool
}

// newVnetClient creates a client transport that dials from the given node.
func newVnetClient(n *vnet.Network, node *vnet.Node, typeName string) *vnetClientTransport {
	ct := &vnetClientTransport{
		net:      n,
		node:     node,
		typeName: typeName,
	}
	return ct
}

func (c *vnetClientTransport) Dial(ctx context.Context, addr string) (transport.Connection, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("%s: transport closed", c.typeName)
	}
	c.mu.Unlock()

	var raw net.Conn
	var err error
	if c.dialFn != nil {
		raw, err = c.dialFn(ctx, addr)
	} else {
		raw, err = c.net.Dial(ctx, c.node, addr)
	}
	if err != nil {
		return nil, fmt.Errorf("%s dial: %w", c.typeName, err)
	}

	cfg := yamux.DefaultConfig()
	cfg.LogOutput = io.Discard
	sess, err := yamux.Client(raw, cfg)
	if err != nil {
		raw.Close()
		return nil, fmt.Errorf("%s yamux client: %w", c.typeName, err)
	}
	return &vnetConn{
		sess:   sess,
		local:  vnetAddr(c.typeName + ":client"),
		remote: vnetAddr(c.typeName + ":server"),
	}, nil
}

func (c *vnetClientTransport) Type() string { return c.typeName }

func (c *vnetClientTransport) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return nil
}

var _ transport.ClientTransport = (*vnetClientTransport)(nil)

// ---------------------------------------------------------------------------
// vnetServerTransport — ServerTransport over vnet
// ---------------------------------------------------------------------------

// vnetServerTransport listens on a vnet node and accepts connections.
type vnetServerTransport struct {
	net      *vnet.Network
	node     *vnet.Node
	typeName string
	addr     string
	listener net.Listener
	mu       sync.Mutex
	closed   bool
}

// newVnetServer creates a server transport that listens on the given node.
func newVnetServer(n *vnet.Network, node *vnet.Node, typeName, addr string) *vnetServerTransport {
	return &vnetServerTransport{
		net:      n,
		node:     node,
		typeName: typeName,
		addr:     addr,
	}
}

func (s *vnetServerTransport) Listen(ctx context.Context) error {
	l, err := s.net.Listen(s.node, s.addr)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.listener = l
	s.mu.Unlock()
	return nil
}

func (s *vnetServerTransport) Accept(ctx context.Context) (transport.Connection, error) {
	s.mu.Lock()
	l := s.listener
	s.mu.Unlock()
	if l == nil {
		return nil, fmt.Errorf("not listening")
	}

	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		c, err := l.Accept()
		ch <- result{c, err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return nil, r.err
		}
		cfg := yamux.DefaultConfig()
		cfg.LogOutput = io.Discard
		sess, err := yamux.Server(r.conn, cfg)
		if err != nil {
			r.conn.Close()
			return nil, err
		}
		return &vnetConn{
			sess:   sess,
			local:  vnetAddr(s.typeName + ":server"),
			remote: vnetAddr(s.typeName + ":client"),
		}, nil
	}
}

func (s *vnetServerTransport) Type() string { return s.typeName }

func (s *vnetServerTransport) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

var _ transport.ServerTransport = (*vnetServerTransport)(nil)

// ---------------------------------------------------------------------------
// echoServer — accepts streams and echoes data back
// ---------------------------------------------------------------------------

// echoServer accepts connections on a server transport and echoes stream data.
// It runs until the context is cancelled.
func echoServer(ctx context.Context, t testing.TB, srv transport.ServerTransport) {
	t.Helper()
	go func() {
		for {
			conn, err := srv.Accept(ctx)
			if err != nil {
				return // context cancelled or listener closed
			}
			go func(c transport.Connection) {
				defer c.Close()
				for {
					stream, err := c.AcceptStream(ctx)
					if err != nil {
						return
					}
					go func(s transport.Stream) {
						defer s.Close()
						io.Copy(s, s)
					}(stream)
				}
			}(conn)
		}
	}()
}
