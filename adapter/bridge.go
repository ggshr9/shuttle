package adapter

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"time"
)

// DialerAsTransport wraps a Dialer as a ClientTransport.
// Each Dial() call returns a single-stream Connection wrapping one net.Conn.
func DialerAsTransport(d Dialer) ClientTransport {
	return &dialerTransport{dialer: d}
}

// TransportAsDialer wraps a ClientTransport as a Dialer.
// Each DialContext call opens a new Connection and returns the first stream as a net.Conn.
func TransportAsDialer(t ClientTransport, serverAddr string) Dialer {
	return &transportDialer{transport: t, serverAddr: serverAddr}
}

// --- dialerTransport ---

type dialerTransport struct {
	dialer Dialer
}

// Dial dials addr using the underlying Dialer. The network is hardcoded to "tcp"
// because Dialer implementations in this package are TCP-based.
func (dt *dialerTransport) Dial(ctx context.Context, addr string) (Connection, error) {
	conn, err := dt.dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	return &singleStreamConn{conn: conn}, nil
}

func (dt *dialerTransport) Type() string { return dt.dialer.Type() }
func (dt *dialerTransport) Close() error { return dt.dialer.Close() }

// --- singleStreamConn wraps a net.Conn as a Connection with exactly one stream ---

type singleStreamConn struct {
	conn   net.Conn
	opened atomic.Bool
}

func (c *singleStreamConn) OpenStream(ctx context.Context) (Stream, error) {
	if !c.opened.CompareAndSwap(false, true) {
		return nil, net.ErrClosed
	}
	return &connStream{conn: c.conn}, nil
}

func (c *singleStreamConn) AcceptStream(ctx context.Context) (Stream, error) {
	// Single-stream conn has no inbound streams; block until context is done.
	<-ctx.Done()
	return nil, ctx.Err()
}

func (c *singleStreamConn) Close() error      { return c.conn.Close() }
func (c *singleStreamConn) LocalAddr() net.Addr  { return c.conn.LocalAddr() }
func (c *singleStreamConn) RemoteAddr() net.Addr { return c.conn.RemoteAddr() }

// --- connStream wraps a net.Conn as a Stream ---

type connStream struct {
	conn net.Conn
}

func (s *connStream) Read(b []byte) (int, error)  { return s.conn.Read(b) }
func (s *connStream) Write(b []byte) (int, error) { return s.conn.Write(b) }
func (s *connStream) Close() error                { return s.conn.Close() }
func (s *connStream) StreamID() uint64            { return 0 }

// --- transportDialer ---

type transportDialer struct {
	transport  ClientTransport
	serverAddr string
}

// DialContext opens a new Connection to the server address bound at construction.
// The network and address parameters are ignored; the serverAddr set in TransportAsDialer is used.
func (td *transportDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	conn, err := td.transport.Dial(ctx, td.serverAddr)
	if err != nil {
		return nil, err
	}
	stream, err := conn.OpenStream(ctx)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &streamConn{stream: stream, conn: conn}, nil
}

func (td *transportDialer) Type() string { return td.transport.Type() }
func (td *transportDialer) Close() error { return td.transport.Close() }

// --- streamConn wraps a Stream as a net.Conn ---

type streamConn struct {
	stream Stream
	conn   Connection
}

func (sc *streamConn) Read(b []byte) (int, error)  { return sc.stream.Read(b) }
func (sc *streamConn) Write(b []byte) (int, error) { return sc.stream.Write(b) }
func (sc *streamConn) Close() error {
	return errors.Join(sc.stream.Close(), sc.conn.Close())
}

func (sc *streamConn) LocalAddr() net.Addr              { return sc.conn.LocalAddr() }
func (sc *streamConn) RemoteAddr() net.Addr             { return sc.conn.RemoteAddr() }
func (sc *streamConn) SetDeadline(_ time.Time) error      { return nil }
func (sc *streamConn) SetReadDeadline(_ time.Time) error  { return nil }
func (sc *streamConn) SetWriteDeadline(_ time.Time) error { return nil }
