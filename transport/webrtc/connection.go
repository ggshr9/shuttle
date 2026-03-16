package webrtc

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/hashicorp/yamux"
	"github.com/pion/webrtc/v4"
	"github.com/shuttleX/shuttle/transport"
)

// wsConn is an abstraction over a WebSocket connection for reconnection signaling.
type wsConn interface {
	io.Closer
}

// wsCloser wraps a close function as a wsConn.
type wsCloser struct {
	closeFn func() error
}

func (w *wsCloser) Close() error { return w.closeFn() }

// webrtcConnection wraps a yamux session over a WebRTC DataChannel.
type webrtcConnection struct {
	mu      sync.RWMutex
	pc      *webrtc.PeerConnection
	session *yamux.Session
	local   net.Addr
	remote  net.Addr
	sc      *statsCollector
	wsConn  wsConn // WebSocket connection for reconnection signaling (nil for HTTP POST path)
}

func (c *webrtcConnection) OpenStream(ctx context.Context) (transport.Stream, error) {
	c.mu.RLock()
	sess := c.session
	c.mu.RUnlock()
	s, err := sess.OpenStream()
	if err != nil {
		return nil, fmt.Errorf("yamux open: %w", err)
	}
	return &webrtcStream{ys: s}, nil
}

func (c *webrtcConnection) AcceptStream(ctx context.Context) (transport.Stream, error) {
	c.mu.RLock()
	sess := c.session
	c.mu.RUnlock()
	s, err := sess.AcceptStream()
	if err != nil {
		return nil, fmt.Errorf("yamux accept: %w", err)
	}
	return &webrtcStream{ys: s}, nil
}

func (c *webrtcConnection) Close() error {
	c.mu.Lock()
	sess := c.session
	pc := c.pc
	sc := c.sc
	ws := c.wsConn
	c.mu.Unlock()

	if sc != nil {
		sc.Close()
	}
	if ws != nil {
		ws.Close()
	}
	sess.Close()
	return pc.Close()
}

// Stats returns the latest connection statistics. Returns zero values if
// the stats collector has not been initialized or no data is available yet.
func (c *webrtcConnection) Stats() ConnStats {
	c.mu.RLock()
	sc := c.sc
	c.mu.RUnlock()
	if sc == nil {
		return ConnStats{}
	}
	return sc.Stats()
}

func (c *webrtcConnection) LocalAddr() net.Addr  { return c.local }
func (c *webrtcConnection) RemoteAddr() net.Addr { return c.remote }

// webrtcStream wraps a yamux.Stream as a transport.Stream.
type webrtcStream struct {
	ys *yamux.Stream
}

func (s *webrtcStream) StreamID() uint64            { return uint64(s.ys.StreamID()) }
func (s *webrtcStream) Read(p []byte) (int, error)  { return s.ys.Read(p) }
func (s *webrtcStream) Write(p []byte) (int, error) { return s.ys.Write(p) }
func (s *webrtcStream) Close() error                { return s.ys.Close() }

// dcReadWriteCloser adapts a detached DataChannel (datachannel.ReadWriteCloser)
// into a plain io.ReadWriteCloser that yamux can use. The detached channel
// from pion already provides a byte-stream interface, so this wrapper mainly
// adds PeerConnection lifecycle awareness.
type dcReadWriteCloser struct {
	rwc    datachanelRWC
	pc     *webrtc.PeerConnection
	closed atomic.Bool
}

// datachanelRWC is the interface returned by DataChannel.Detach().
type datachanelRWC interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Close() error
}

func (d *dcReadWriteCloser) Read(p []byte) (int, error) {
	return d.rwc.Read(p)
}

func (d *dcReadWriteCloser) Write(p []byte) (int, error) {
	return d.rwc.Write(p)
}

func (d *dcReadWriteCloser) Close() error {
	if d.closed.CompareAndSwap(false, true) {
		return d.rwc.Close()
	}
	return nil
}

// webrtcAddr implements net.Addr for WebRTC connections.
type webrtcAddr struct {
	addr string
}

func (a *webrtcAddr) Network() string { return "webrtc" }
func (a *webrtcAddr) String() string  { return a.addr }

// Compile-time interface checks.
var _ transport.Connection = (*webrtcConnection)(nil)
var _ transport.Stream = (*webrtcStream)(nil)
