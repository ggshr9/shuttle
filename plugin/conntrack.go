package plugin

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// ConnEmitter is an interface for emitting connection events.
// This is typically implemented by the engine.
type ConnEmitter interface {
	EmitConnectionEvent(connID, state, target, rule, protocol, processName string, bytesIn, bytesOut, durationMs int64)
}

// ConnTracker is a plugin that tracks connection lifecycle and emits events.
type ConnTracker struct {
	emitter ConnEmitter
	counter atomic.Uint64
	conns   sync.Map // connID -> *trackedConn
}

type trackedConn struct {
	id        string
	target    string
	rule      string
	protocol  string
	process   string
	startTime time.Time
	bytesIn   atomic.Int64
	bytesOut  atomic.Int64
	conn      net.Conn
}

// NewConnTracker creates a new connection tracker plugin.
func NewConnTracker(emitter ConnEmitter) *ConnTracker {
	return &ConnTracker{emitter: emitter}
}

func (ct *ConnTracker) Name() string                    { return "conntrack" }
func (ct *ConnTracker) Init(ctx context.Context) error  { return nil }
func (ct *ConnTracker) Close() error                    { return nil }

func (ct *ConnTracker) OnConnect(conn net.Conn, target string) (net.Conn, error) {
	id := ct.counter.Add(1)
	connID := formatConnID(id)

	tc := &trackedConn{
		id:        connID,
		target:    target,
		protocol:  detectProtocol(conn),
		startTime: time.Now(),
		conn:      conn,
	}

	// Try to extract process name and rule from context if available
	// These would be set by the router/dialer
	if ctx := conn.RemoteAddr(); ctx != nil {
		// Process name might be available via context in TUN mode
		tc.process = "" // Will be populated if available
	}

	ct.conns.Store(connID, tc)

	// Emit connection opened event
	if ct.emitter != nil {
		ct.emitter.EmitConnectionEvent(
			connID, "opened", target, tc.rule, tc.protocol, tc.process,
			0, 0, 0,
		)
	}

	// Wrap the connection to track bytes and detect close
	return &trackingConn{
		Conn:    conn,
		tracked: tc,
		tracker: ct,
	}, nil
}

func (ct *ConnTracker) OnDisconnect(conn net.Conn) {
	// Find and remove the tracked connection
	if tc, ok := conn.(*trackingConn); ok {
		ct.closeTracked(tc.tracked)
	}
}

func (ct *ConnTracker) closeTracked(tc *trackedConn) {
	ct.conns.Delete(tc.id)

	duration := time.Since(tc.startTime).Milliseconds()

	if ct.emitter != nil {
		ct.emitter.EmitConnectionEvent(
			tc.id, "closed", tc.target, tc.rule, tc.protocol, tc.process,
			tc.bytesIn.Load(), tc.bytesOut.Load(), duration,
		)
	}
}

// SetConnRule sets the matched rule for a connection.
func (ct *ConnTracker) SetConnRule(connID, rule string) {
	if v, ok := ct.conns.Load(connID); ok {
		tc := v.(*trackedConn)
		tc.rule = rule
	}
}

// SetConnProcess sets the process name for a connection.
func (ct *ConnTracker) SetConnProcess(connID, process string) {
	if v, ok := ct.conns.Load(connID); ok {
		tc := v.(*trackedConn)
		tc.process = process
	}
}

func formatConnID(id uint64) string {
	const chars = "0123456789abcdef"
	buf := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		buf[i] = chars[id&0xf]
		id >>= 4
	}
	return string(buf)
}

func detectProtocol(conn net.Conn) string {
	addr := conn.RemoteAddr()
	if addr == nil {
		return "unknown"
	}
	switch addr.Network() {
	case "tcp", "tcp4", "tcp6":
		return "tcp"
	case "udp", "udp4", "udp6":
		return "udp"
	default:
		return addr.Network()
	}
}

// trackingConn wraps a net.Conn to track bytes transferred.
type trackingConn struct {
	net.Conn
	tracked *trackedConn
	tracker *ConnTracker
	closed  atomic.Bool
}

func (tc *trackingConn) Read(b []byte) (n int, err error) {
	n, err = tc.Conn.Read(b)
	if n > 0 {
		tc.tracked.bytesIn.Add(int64(n))
	}
	return
}

func (tc *trackingConn) Write(b []byte) (n int, err error) {
	n, err = tc.Conn.Write(b)
	if n > 0 {
		tc.tracked.bytesOut.Add(int64(n))
	}
	return
}

func (tc *trackingConn) Close() error {
	if tc.closed.CompareAndSwap(false, true) {
		tc.tracker.closeTracked(tc.tracked)
	}
	return tc.Conn.Close()
}

var _ ConnPlugin = (*ConnTracker)(nil)
