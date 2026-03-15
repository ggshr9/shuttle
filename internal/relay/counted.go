package relay

import (
	"io"
	"sync/atomic"
)

// CountedReadWriter wraps an io.ReadWriteCloser and counts bytes
// transferred in each direction.
//
// Note: this wrapper intentionally does NOT implement io.ReaderFrom or
// io.WriterTo. On Linux, those interfaces trigger splice(2) which bypasses
// userspace entirely — making byte counting impossible. When counting is
// not needed, use the raw connection directly to get zero-copy benefits.
type CountedReadWriter struct {
	inner    io.ReadWriteCloser
	sent     atomic.Int64
	received atomic.Int64
}

// NewCountedReadWriter wraps inner with byte counting.
func NewCountedReadWriter(inner io.ReadWriteCloser) *CountedReadWriter {
	return &CountedReadWriter{inner: inner}
}

// Read implements io.Reader and counts bytes received (read from inner).
func (c *CountedReadWriter) Read(p []byte) (int, error) {
	n, err := c.inner.Read(p)
	if n > 0 {
		c.received.Add(int64(n))
	}
	return n, err
}

// Write implements io.Writer and counts bytes sent (written to inner).
func (c *CountedReadWriter) Write(p []byte) (int, error) {
	n, err := c.inner.Write(p)
	if n > 0 {
		c.sent.Add(int64(n))
	}
	return n, err
}

// Close closes the underlying connection.
func (c *CountedReadWriter) Close() error {
	return c.inner.Close()
}

// CloseWrite forwards half-close to the inner connection if supported.
// This enables Relay to signal EOF in one direction without closing the read side.
func (c *CountedReadWriter) CloseWrite() error {
	if cw, ok := c.inner.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return c.inner.Close()
}

// Sent returns total bytes written to the inner connection.
func (c *CountedReadWriter) Sent() int64 {
	return c.sent.Load()
}

// Received returns total bytes read from the inner connection.
func (c *CountedReadWriter) Received() int64 {
	return c.received.Load()
}
