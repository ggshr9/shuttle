package conformance

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/yamux"
	"github.com/shuttleX/shuttle/transport"
)

// pipeTransport is a reference transport implementation backed by net.Pipe()
// and yamux multiplexing. It is used to self-test the conformance suite.

// --- Stream wrapper ---

type pipeStream struct {
	ys *yamux.Stream
	id uint64
}

func (s *pipeStream) Read(p []byte) (int, error)  { return s.ys.Read(p) }
func (s *pipeStream) Write(p []byte) (int, error)  { return s.ys.Write(p) }
func (s *pipeStream) Close() error                  { return s.ys.Close() }
func (s *pipeStream) StreamID() uint64               { return s.id }

var _ transport.Stream = (*pipeStream)(nil)

// --- Connection wrapper ---

type pipeConn struct {
	sess      *yamux.Session
	local     net.Addr
	remote    net.Addr
	streamSeq atomic.Uint64
}

func (c *pipeConn) OpenStream(ctx context.Context) (transport.Stream, error) {
	ys, err := c.sess.OpenStream()
	if err != nil {
		return nil, err
	}
	id := c.streamSeq.Add(1)
	return &pipeStream{ys: ys, id: id}, nil
}

func (c *pipeConn) AcceptStream(ctx context.Context) (transport.Stream, error) {
	// yamux doesn't take a context; wrap with a goroutine.
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
		return &pipeStream{ys: r.s, id: id}, nil
	}
}

func (c *pipeConn) Close() error      { return c.sess.Close() }
func (c *pipeConn) LocalAddr() net.Addr  { return c.local }
func (c *pipeConn) RemoteAddr() net.Addr { return c.remote }

var _ transport.Connection = (*pipeConn)(nil)

// --- Client transport ---

type pipeClient struct {
	mu   sync.Mutex
	pipe net.Conn // client end of the pipe
	used bool
	closed atomic.Bool
}

func (c *pipeClient) Dial(ctx context.Context, addr string) (transport.Connection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.closed.Load() {
		return nil, fmt.Errorf("pipe client closed")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.used {
		return nil, fmt.Errorf("pipe client: only one connection supported")
	}
	c.used = true

	cfg := yamux.DefaultConfig()
	cfg.LogOutput = io.Discard
	sess, err := yamux.Client(c.pipe, cfg)
	if err != nil {
		return nil, fmt.Errorf("yamux client: %w", err)
	}
	return &pipeConn{
		sess:   sess,
		local:  pipeAddr("pipe:client"),
		remote: pipeAddr("pipe:server"),
	}, nil
}

func (c *pipeClient) Type() string { return "pipe" }

func (c *pipeClient) Close() error {
	c.closed.Store(true)
	return c.pipe.Close()
}

var _ transport.ClientTransport = (*pipeClient)(nil)

// --- Server transport ---

type pipeServer struct {
	pipe   net.Conn // server end of the pipe
	connCh chan transport.Connection
	closed atomic.Bool
}

func (s *pipeServer) Listen(ctx context.Context) error {
	// Create a yamux session over the pipe.
	cfg := yamux.DefaultConfig()
	cfg.LogOutput = io.Discard
	sess, err := yamux.Server(s.pipe, cfg)
	if err != nil {
		return fmt.Errorf("yamux server: %w", err)
	}
	conn := &pipeConn{
		sess:   sess,
		local:  pipeAddr("pipe:server"),
		remote: pipeAddr("pipe:client"),
	}
	// Deliver exactly one connection.
	go func() {
		s.connCh <- conn
	}()
	return nil
}

func (s *pipeServer) Accept(ctx context.Context) (transport.Connection, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case c := <-s.connCh:
		return c, nil
	}
}

func (s *pipeServer) Type() string { return "pipe" }

func (s *pipeServer) Close() error {
	s.closed.Store(true)
	return s.pipe.Close()
}

var _ transport.ServerTransport = (*pipeServer)(nil)

// --- Address type ---

type pipeAddr string

func (a pipeAddr) Network() string { return "pipe" }
func (a pipeAddr) String() string  { return string(a) }

// --- Factory ---

func pipeFactory(t testing.TB) (
	client transport.ClientTransport,
	server transport.ServerTransport,
	serverAddr string,
	cleanup func(),
) {
	t.Helper()
	clientEnd, serverEnd := net.Pipe()
	c := &pipeClient{pipe: clientEnd}
	s := &pipeServer{
		pipe:   serverEnd,
		connCh: make(chan transport.Connection, 1),
	}
	return c, s, "pipe", func() {
		c.Close()
		s.Close()
	}
}

// TestPipeConformance runs the full conformance suite against the pipe transport.
func TestPipeConformance(t *testing.T) {
	RunSuite(t, pipeFactory)
}
