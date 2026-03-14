package fault

import (
	"net"
	"time"
)

// faultConn wraps a net.Conn and applies fault injection rules on reads and writes.
type faultConn struct {
	inner    net.Conn
	injector *Injector
}

var _ net.Conn = (*faultConn)(nil)

func (c *faultConn) Read(b []byte) (int, error) {
	n, err := c.inner.Read(b)
	if err != nil {
		return n, err
	}
	out, faultErr, matched := c.injector.applyRules(c.injector.readRules, b[:n])
	if !matched {
		return n, nil
	}
	if faultErr != nil {
		return 0, faultErr
	}
	// drop sentinel: nil data, nil error
	if out == nil {
		return n, nil // pass through on read drop (data already consumed)
	}
	copy(b, out)
	return len(out), nil
}

func (c *faultConn) Write(b []byte) (int, error) {
	out, faultErr, matched := c.injector.applyRules(c.injector.writeRules, b)
	if !matched {
		return c.inner.Write(b)
	}
	if faultErr != nil {
		return 0, faultErr
	}
	// drop sentinel: nil data, nil error — report success but don't forward
	if out == nil {
		return len(b), nil
	}
	return c.inner.Write(out)
}

func (c *faultConn) Close() error                       { return c.inner.Close() }
func (c *faultConn) LocalAddr() net.Addr                { return c.inner.LocalAddr() }
func (c *faultConn) RemoteAddr() net.Addr               { return c.inner.RemoteAddr() }
func (c *faultConn) SetDeadline(t time.Time) error      { return c.inner.SetDeadline(t) }
func (c *faultConn) SetReadDeadline(t time.Time) error  { return c.inner.SetReadDeadline(t) }
func (c *faultConn) SetWriteDeadline(t time.Time) error { return c.inner.SetWriteDeadline(t) }
