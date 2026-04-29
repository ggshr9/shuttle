package engine

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/ggshr9/shuttle/obfs"
	"github.com/ggshr9/shuttle/plugin"
	"github.com/ggshr9/shuttle/transport"
)

// chainConn wraps a net.Conn so that closing it calls chain.OnDisconnect,
// ensuring all plugins in the chain (metrics, logger, etc.) are notified.
type chainConn struct {
	net.Conn
	chain     *plugin.Chain
	closeOnce sync.Once
}

func (c *chainConn) Close() error {
	c.closeOnce.Do(func() {
		c.chain.OnDisconnect(c.Conn)
	})
	return c.Conn.Close()
}

// shapedConn wraps a streamConn so that writes go through an obfs.Shaper
// (randomized chunking and inter-packet delays) while reads pass through unchanged.
type shapedConn struct {
	*streamConn
	shaper *obfs.Shaper
}

func (c *shapedConn) Write(b []byte) (int, error) { return c.shaper.Write(b) }

// ReadFrom disables zero-copy so that writes always go through the Shaper.
func (c *shapedConn) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(struct{ io.Writer }{c}, r)
}

// streamConn wraps a transport.Stream as a net.Conn.
type streamConn struct {
	stream transport.Stream
	addr   string
}

func (c *streamConn) Read(b []byte) (int, error)         { return c.stream.Read(b) }
func (c *streamConn) Write(b []byte) (int, error)        { return c.stream.Write(b) }
func (c *streamConn) Close() error                        { return c.stream.Close() }
func (c *streamConn) LocalAddr() net.Addr                 { return &net.TCPAddr{} }
func (c *streamConn) RemoteAddr() net.Addr                { return &net.TCPAddr{} }
func (c *streamConn) SetDeadline(t time.Time) error       { return nil }
func (c *streamConn) SetReadDeadline(t time.Time) error   { return nil }
func (c *streamConn) SetWriteDeadline(t time.Time) error  { return nil }

// ReadFrom delegates to the underlying stream's ReadFrom if available,
// preserving zero-copy (splice) capability on Linux.
func (c *streamConn) ReadFrom(r io.Reader) (int64, error) {
	if rf, ok := c.stream.(io.ReaderFrom); ok {
		return rf.ReadFrom(r)
	}
	return io.Copy(struct{ io.Writer }{c.stream}, r)
}

// WriteTo delegates to the underlying stream's WriteTo if available,
// preserving zero-copy (splice) capability on Linux.
func (c *streamConn) WriteTo(w io.Writer) (int64, error) {
	if wt, ok := c.stream.(io.WriterTo); ok {
		return wt.WriteTo(w)
	}
	return io.Copy(w, struct{ io.Reader }{c.stream})
}

// extractPort parses a host:port address and returns the port as uint16.
// Returns 0 if the address is malformed or the port is out of range.
func extractPort(addr string) uint16 {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	var port int
	for _, c := range portStr {
		if c < '0' || c > '9' {
			return 0
		}
		port = port*10 + int(c-'0')
		if port > 65535 {
			return 0
		}
	}
	return uint16(port)
}
