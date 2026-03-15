package test

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/shuttle-proxy/shuttle/congestion"
	"github.com/shuttle-proxy/shuttle/transport"
	"github.com/shuttle-proxy/shuttle/transport/selector"
)

// ---------------------------------------------------------------------------
// BenchmarkStreamWriteRead – measure raw stream throughput via io.Pipe
// ---------------------------------------------------------------------------

func BenchmarkStreamWriteRead(b *testing.B) {
	for _, size := range []int{1 << 10, 64 << 10, 1 << 20} {
		name := formatSize(size)
		b.Run(name, func(b *testing.B) {
			data := make([]byte, size)
			buf := make([]byte, size)
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pr, pw := io.Pipe()
				go func() {
					pw.Write(data) //nolint:errcheck
					pw.Close()
				}()
				io.ReadFull(pr, buf) //nolint:errcheck
				pr.Close()
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BenchmarkYamuxSession – stream creation + data transfer over yamux
// ---------------------------------------------------------------------------

func BenchmarkYamuxSession(b *testing.B) {
	for _, size := range []int{1 << 10, 64 << 10, 1 << 20} {
		name := formatSize(size)
		b.Run(name, func(b *testing.B) {
			client, server := yamuxPair(b)
			defer client.Close()
			defer server.Close()

			data := make([]byte, size)
			buf := make([]byte, size)
			b.SetBytes(int64(size))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup
				wg.Add(1)
				go func() {
					defer wg.Done()
					s, err := server.AcceptStream()
					if err != nil {
						return
					}
					io.ReadFull(s, buf) //nolint:errcheck
					s.Close()
				}()

				stream, err := client.OpenStream()
				if err != nil {
					b.Fatal(err)
				}
				stream.Write(data) //nolint:errcheck
				stream.Close()
				wg.Wait()
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BenchmarkConcurrentStreams – aggregate throughput with N concurrent yamux streams
// ---------------------------------------------------------------------------

func BenchmarkConcurrentStreams(b *testing.B) {
	for _, n := range []int{1, 4, 16, 64} {
		b.Run(fmt.Sprintf("streams-%d", n), func(b *testing.B) {
			client, server := yamuxPair(b)
			defer client.Close()
			defer server.Close()

			const payloadSize = 64 << 10
			data := make([]byte, payloadSize)
			b.SetBytes(int64(payloadSize) * int64(n))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup

				// Server side: accept n streams and read fully.
				wg.Add(n)
				for j := 0; j < n; j++ {
					go func() {
						defer wg.Done()
						s, err := server.AcceptStream()
						if err != nil {
							return
						}
						io.Copy(io.Discard, s) //nolint:errcheck
						s.Close()
					}()
				}

				// Client side: open n streams concurrently and write.
				var cwg sync.WaitGroup
				cwg.Add(n)
				for j := 0; j < n; j++ {
					go func() {
						defer cwg.Done()
						s, err := client.OpenStream()
						if err != nil {
							return
						}
						s.Write(data) //nolint:errcheck
						s.Close()
					}()
				}
				cwg.Wait()
				wg.Wait()
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BenchmarkSelectorDial – selector Dial with fake transports
// ---------------------------------------------------------------------------

// benchFakeStream implements transport.Stream for benchmarks.
type benchFakeStream struct{}

func (s *benchFakeStream) Read(b []byte) (int, error)  { return len(b), nil }
func (s *benchFakeStream) Write(b []byte) (int, error) { return len(b), nil }
func (s *benchFakeStream) Close() error                { return nil }
func (s *benchFakeStream) StreamID() uint64            { return 1 }

// benchFakeConn implements transport.Connection for benchmarks.
type benchFakeConn struct{}

func (c *benchFakeConn) OpenStream(ctx context.Context) (transport.Stream, error) {
	return &benchFakeStream{}, nil
}
func (c *benchFakeConn) AcceptStream(ctx context.Context) (transport.Stream, error) {
	return &benchFakeStream{}, nil
}
func (c *benchFakeConn) Close() error        { return nil }
func (c *benchFakeConn) LocalAddr() net.Addr  { return &net.TCPAddr{} }
func (c *benchFakeConn) RemoteAddr() net.Addr { return &net.TCPAddr{} }

// benchFakeTransport implements transport.ClientTransport for benchmarks.
type benchFakeTransport struct {
	name string
	conn transport.Connection
}

func (t *benchFakeTransport) Dial(ctx context.Context, addr string) (transport.Connection, error) {
	return t.conn, nil
}
func (t *benchFakeTransport) Type() string { return t.name }
func (t *benchFakeTransport) Close() error { return nil }

func BenchmarkSelectorDial(b *testing.B) {
	transports := []transport.ClientTransport{
		&benchFakeTransport{name: "h3", conn: &benchFakeConn{}},
		&benchFakeTransport{name: "reality", conn: &benchFakeConn{}},
		&benchFakeTransport{name: "cdn", conn: &benchFakeConn{}},
	}

	sel := selector.New(transports, &selector.Config{Strategy: selector.StrategyPriority}, nil)

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn, err := sel.Dial(ctx, "bench:443")
		if err != nil {
			b.Fatal(err)
		}
		_ = conn
	}
}

// ---------------------------------------------------------------------------
// BenchmarkAdaptiveCongestionUnderLoad – mixed OnAck/OnPacketSent/OnPacketLoss
// ---------------------------------------------------------------------------

func BenchmarkAdaptiveCongestionUnderLoad(b *testing.B) {
	ac := congestion.NewAdaptive(&congestion.AdaptiveConfig{
		BrutalRate:     100 * 1024 * 1024,
		LossThreshold:  0.05,
		SwitchCooldown: 1 * time.Millisecond, // fast cooldown for benchmark
	}, nil)

	rtt := 50 * time.Millisecond
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate realistic packet flow: 80% ack, 15% send, 5% loss.
		switch i % 20 {
		case 0:
			ac.OnPacketLoss(1200)
		case 1, 2, 3:
			ac.OnPacketSent(1200)
		default:
			ac.OnAck(1200, rtt)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func yamuxPair(tb testing.TB) (*yamux.Session, *yamux.Session) {
	tb.Helper()
	c1, c2 := net.Pipe()
	conf := yamux.DefaultConfig()
	conf.LogOutput = io.Discard

	clientCh := make(chan *yamux.Session, 1)
	errCh := make(chan error, 1)
	go func() {
		s, err := yamux.Client(c1, conf)
		if err != nil {
			errCh <- err
			return
		}
		clientCh <- s
	}()

	server, err := yamux.Server(c2, conf)
	if err != nil {
		tb.Fatal(err)
	}

	select {
	case client := <-clientCh:
		return client, server
	case err := <-errCh:
		tb.Fatal(err)
		return nil, nil
	}
}

func formatSize(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%dMB", n>>20)
	case n >= 1<<10:
		return fmt.Sprintf("%dKB", n>>10)
	default:
		return fmt.Sprintf("%dB", n)
	}
}
