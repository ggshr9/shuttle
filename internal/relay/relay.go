// Package relay provides bidirectional data copying between two connections.
// It attempts to preserve zero-copy capabilities (e.g., splice on Linux)
// by delegating io.ReaderFrom / io.WriterTo when the underlying types support them.
package relay

import (
	"io"
	"log/slog"
	"sync"

	"github.com/shuttleX/shuttle/internal/pool"
)

// Relay copies data bidirectionally between a and b.
// It attempts to use platform-optimized zero-copy (splice on Linux)
// when both sides support it, falling back to io.CopyBuffer with
// pooled buffers otherwise.
//
// Returns the total bytes transferred in each direction and the first error.
func Relay(a, b io.ReadWriteCloser) (sent, received int64, err error) {
	// Try zero-copy splice first (Linux only, raw TCP connections).
	if n1, n2, ok := trySplice(a, b); ok {
		return n1, n2, nil
	}

	// Fallback to userspace copy with pooled buffers.
	var (
		wg      sync.WaitGroup
		errOnce sync.Once
		aToB    int64
		bToA    int64
	)

	wg.Add(2)

	// a → b
	go func() {
		defer wg.Done()
		n, copyErr := copyBuffer(b, a)
		aToB = n
		if copyErr != nil {
			errOnce.Do(func() { err = copyErr })
			slog.Debug("relay a→b finished", "err", copyErr)
		}
		// Signal EOF to the write side so the other direction can finish.
		// Prefer CloseWrite to keep the read side open; fall back to full Close.
		if cw, ok := b.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		} else {
			_ = b.Close()
		}
	}()

	// b → a
	go func() {
		defer wg.Done()
		n, copyErr := copyBuffer(a, b)
		bToA = n
		if copyErr != nil {
			errOnce.Do(func() { err = copyErr })
			slog.Debug("relay b→a finished", "err", copyErr)
		}
		if cw, ok := a.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		} else {
			_ = a.Close()
		}
	}()

	wg.Wait()
	return aToB, bToA, err
}

// copyBuffer copies from src to dst using a pooled buffer.
// If dst implements io.ReaderFrom or src implements io.WriterTo,
// Go's io.Copy will use those interfaces (which enables splice on Linux
// for *net.TCPConn). We only fall back to io.CopyBuffer with an explicit
// buffer when neither zero-copy interface is available.
func copyBuffer(dst io.Writer, src io.Reader) (int64, error) {
	// Check if either side supports zero-copy interfaces.
	// If so, let io.Copy handle it — it will detect ReaderFrom/WriterTo.
	if _, ok := dst.(io.ReaderFrom); ok {
		return io.Copy(dst, src)
	}
	if _, ok := src.(io.WriterTo); ok {
		return io.Copy(dst, src)
	}

	// Fall back to pooled 32KB buffer (exact MedLarge tier, no waste).
	// Use NoZero variant — relay buffers contain proxied traffic, not secrets.
	buf := pool.GetMedLarge()
	n, err := io.CopyBuffer(dst, src, buf)
	pool.PutMedLargeNoZero(buf)
	return n, err
}
