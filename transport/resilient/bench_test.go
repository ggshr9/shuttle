package resilient

import (
	"context"
	"testing"
	"time"

	"github.com/ggshr9/shuttle/transport"
)

// BenchmarkResilientConnOpenStream benchmarks OpenStream on a healthy connection
// with no reconnection needed.
func BenchmarkResilientConnOpenStream(b *testing.B) {
	conn := newHealthyConn()
	dial := func(ctx context.Context) (transport.Connection, error) {
		return newHealthyConn(), nil
	}
	rc := Wrap(conn, dial, Config{})

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, err := rc.OpenStream(ctx)
		if err != nil {
			b.Fatal(err)
		}
		_ = s
	}
}

// BenchmarkKeepaliveCheck benchmarks the IsHealthy check which reads an atomic bool.
func BenchmarkKeepaliveCheck(b *testing.B) {
	conn := newHealthyConn()
	dial := func(ctx context.Context) (transport.Connection, error) {
		return newHealthyConn(), nil
	}
	rc := Wrap(conn, dial, Config{})
	// Set healthy state without starting the keepalive loop.
	rc.healthy.Store(true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rc.IsHealthy()
	}
}

// BenchmarkStaleDetectorTrack benchmarks RecordSuccess calls on a StaleDetector.
func BenchmarkStaleDetectorTrack(b *testing.B) {
	conn := newHealthyConn()
	sd := &StaleDetector{
		conn:        conn,
		maxIdle:     30 * time.Second,
		onStale:     func() {},
		lastSuccess: time.Now(),
	}
	// Don't start the loop — we only benchmark RecordSuccess.
	sd.cancel = func() {} // no-op cancel to avoid nil panic on cleanup

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sd.RecordSuccess()
	}
}
