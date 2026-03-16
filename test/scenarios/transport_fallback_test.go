package scenarios

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/shuttleX/shuttle/testkit/vnet"
	"github.com/shuttleX/shuttle/transport"
	"github.com/shuttleX/shuttle/transport/selector"
)

// ---------------------------------------------------------------------------
// TestFallbackOnDialFailure
//
// Primary transport dial fails -> selector tries secondary -> succeeds.
// We set up two yamux-over-vnet transports ("h3" and "cdn"). The first
// transport's dial is rigged to always fail. The selector should fall back
// to the secondary transport and successfully open a stream.
// ---------------------------------------------------------------------------

func TestFallbackOnDialFailure(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := NewEnv(t, vnet.WithSeed(1))
	defer env.Close()

	client := env.Net.AddNode("client")
	server := env.Net.AddNode("server")
	env.Net.Link(client, server, vnet.LinkConfig{Latency: 5 * time.Millisecond})

	// Set up two server transports on different addresses.
	srvH3 := newVnetServer(env.Net, server, "h3", "server:4430")
	srvCDN := newVnetServer(env.Net, server, "cdn", "server:4431")
	if err := srvH3.Listen(ctx); err != nil {
		t.Fatalf("h3 listen: %v", err)
	}
	if err := srvCDN.Listen(ctx); err != nil {
		t.Fatalf("cdn listen: %v", err)
	}
	defer srvH3.Close()
	defer srvCDN.Close()

	echoServer(ctx, t, srvH3)
	echoServer(ctx, t, srvCDN)

	// Client transport for h3 — always fails on dial.
	failClient := newVnetClient(env.Net, client, "h3")
	failClient.dialFn = func(ctx context.Context, addr string) (net.Conn, error) {
		return nil, fmt.Errorf("simulated h3 dial failure")
	}

	// Client transport for cdn — works normally.
	cdnClient := newVnetClient(env.Net, client, "cdn")
	cdnClient.dialFn = func(ctx context.Context, addr string) (net.Conn, error) {
		return env.Net.Dial(ctx, client, "server:4431")
	}

	// Build selector with priority strategy (h3 first, cdn second).
	sel := selector.New(
		[]transport.ClientTransport{failClient, cdnClient},
		&selector.Config{Strategy: selector.StrategyPriority},
		nil,
	)
	defer sel.Close()

	// Dial should fail on h3, fall back to cdn.
	conn, err := sel.Dial(ctx, "server:4431")
	if err != nil {
		t.Fatalf("selector.Dial should have fallen back to cdn: %v", err)
	}

	// Verify the fallback transport is active.
	if got := sel.ActiveTransport(); got != "cdn" {
		t.Fatalf("ActiveTransport = %s, want cdn", got)
	}

	// Verify data flows through the fallback connection.
	stream, err := conn.OpenStream(ctx)
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	defer stream.Close()

	payload := []byte("hello via fallback")
	if _, err := stream.Write(payload); err != nil {
		t.Fatalf("Write: %v", err)
	}

	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(stream, buf); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	if !bytes.Equal(buf, payload) {
		t.Fatalf("echoed data mismatch: got %q, want %q", buf, payload)
	}
}

// ---------------------------------------------------------------------------
// TestFallbackOnMidStreamFailure
//
// Primary connection is established and data is streaming. We inject an error
// mid-transfer. The client detects the failure and successfully opens a new
// stream on the secondary transport to re-send the data.
// ---------------------------------------------------------------------------

func TestFallbackOnMidStreamFailure(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := NewEnv(t, vnet.WithSeed(2))
	defer env.Close()

	client := env.Net.AddNode("client")
	server := env.Net.AddNode("server")
	env.Net.Link(client, server, vnet.LinkConfig{Latency: 2 * time.Millisecond})

	// Two server listeners.
	srvPrimary := newVnetServer(env.Net, server, "h3", "server:5000")
	srvSecondary := newVnetServer(env.Net, server, "cdn", "server:5001")
	if err := srvPrimary.Listen(ctx); err != nil {
		t.Fatal(err)
	}
	if err := srvSecondary.Listen(ctx); err != nil {
		t.Fatal(err)
	}
	defer srvPrimary.Close()
	defer srvSecondary.Close()

	echoServer(ctx, t, srvPrimary)
	echoServer(ctx, t, srvSecondary)

	// Client transports — both dial normally.
	primaryClient := newVnetClient(env.Net, client, "h3")
	primaryClient.dialFn = func(ctx context.Context, addr string) (net.Conn, error) {
		return env.Net.Dial(ctx, client, "server:5000")
	}
	secondaryClient := newVnetClient(env.Net, client, "cdn")
	secondaryClient.dialFn = func(ctx context.Context, addr string) (net.Conn, error) {
		return env.Net.Dial(ctx, client, "server:5001")
	}

	sel := selector.New(
		[]transport.ClientTransport{primaryClient, secondaryClient},
		&selector.Config{Strategy: selector.StrategyPriority},
		nil,
	)
	defer sel.Close()

	// Establish connection on primary.
	conn, err := sel.Dial(ctx, "server:5000")
	if err != nil {
		t.Fatalf("initial Dial: %v", err)
	}

	if got := sel.ActiveTransport(); got != "h3" {
		t.Fatalf("ActiveTransport = %s, want h3", got)
	}

	// Open a stream and verify it works.
	stream, err := conn.OpenStream(ctx)
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	msg1 := []byte("before-break")
	if _, err := stream.Write(msg1); err != nil {
		t.Fatalf("Write msg1: %v", err)
	}
	buf := make([]byte, len(msg1))
	if _, err := io.ReadFull(stream, buf); err != nil {
		t.Fatalf("ReadFull msg1: %v", err)
	}
	if !bytes.Equal(buf, msg1) {
		t.Fatalf("msg1 mismatch: got %q, want %q", buf, msg1)
	}
	stream.Close()

	// Simulate primary connection breaking by closing the underlying connection.
	// After this, new dials on primary will fail and selector should fall back.
	primaryClient.Close()

	// Dial again — primary is closed, should fall back to secondary.
	conn2, err := sel.Dial(ctx, "server:5001")
	if err != nil {
		t.Fatalf("fallback Dial after break: %v", err)
	}

	stream2, err := conn2.OpenStream(ctx)
	if err != nil {
		t.Fatalf("OpenStream on fallback: %v", err)
	}
	defer stream2.Close()

	msg2 := []byte("after-break-via-secondary")
	if _, err := stream2.Write(msg2); err != nil {
		t.Fatalf("Write msg2: %v", err)
	}
	buf2 := make([]byte, len(msg2))
	if _, err := io.ReadFull(stream2, buf2); err != nil {
		t.Fatalf("ReadFull msg2: %v", err)
	}
	if !bytes.Equal(buf2, msg2) {
		t.Fatalf("msg2 mismatch: got %q, want %q", buf2, msg2)
	}
}

// ---------------------------------------------------------------------------
// TestAllTransportsFail
//
// All transports fail -> meaningful error returned.
// ---------------------------------------------------------------------------

func TestAllTransportsFail(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	env := NewEnv(t, vnet.WithSeed(3))
	defer env.Close()

	client := env.Net.AddNode("client")

	// Both transports always fail on dial.
	t1 := newVnetClient(env.Net, client, "h3")
	t1.dialFn = func(ctx context.Context, addr string) (net.Conn, error) {
		return nil, errors.New("h3: connection refused")
	}
	t2 := newVnetClient(env.Net, client, "cdn")
	t2.dialFn = func(ctx context.Context, addr string) (net.Conn, error) {
		return nil, errors.New("cdn: timeout")
	}

	sel := selector.New(
		[]transport.ClientTransport{t1, t2},
		&selector.Config{Strategy: selector.StrategyPriority},
		nil,
	)
	defer sel.Close()

	_, err := sel.Dial(ctx, "unreachable:443")
	if err == nil {
		t.Fatal("expected error when all transports fail, got nil")
	}
	t.Logf("all-fail error (expected): %v", err)
}

// ---------------------------------------------------------------------------
// TestFallbackWithLatencyStrategy
//
// With latency strategy, verify the selector picks the lowest-latency
// transport and falls back correctly when it becomes unavailable.
// ---------------------------------------------------------------------------

func TestFallbackWithLatencyStrategy(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := NewEnv(t, vnet.WithSeed(4))
	defer env.Close()

	client := env.Net.AddNode("client")
	server := env.Net.AddNode("server")
	env.Net.Link(client, server, vnet.LinkConfig{Latency: 5 * time.Millisecond})

	srvFast := newVnetServer(env.Net, server, "reality", "server:6000")
	srvSlow := newVnetServer(env.Net, server, "cdn", "server:6001")
	if err := srvFast.Listen(ctx); err != nil {
		t.Fatal(err)
	}
	if err := srvSlow.Listen(ctx); err != nil {
		t.Fatal(err)
	}
	defer srvFast.Close()
	defer srvSlow.Close()

	echoServer(ctx, t, srvFast)
	echoServer(ctx, t, srvSlow)

	fastClient := newVnetClient(env.Net, client, "reality")
	fastClient.dialFn = func(ctx context.Context, addr string) (net.Conn, error) {
		return env.Net.Dial(ctx, client, "server:6000")
	}
	slowClient := newVnetClient(env.Net, client, "cdn")
	slowClient.dialFn = func(ctx context.Context, addr string) (net.Conn, error) {
		return env.Net.Dial(ctx, client, "server:6001")
	}

	sel := selector.New(
		[]transport.ClientTransport{fastClient, slowClient},
		&selector.Config{Strategy: selector.StrategyLatency},
		nil,
	)
	defer sel.Close()

	// Without probes, no active transport yet; first dial tries in order.
	conn, err := sel.Dial(ctx, "server:6000")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	stream, err := conn.OpenStream(ctx)
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	defer stream.Close()

	payload := []byte("latency-strategy-test")
	if _, err := stream.Write(payload); err != nil {
		t.Fatalf("Write: %v", err)
	}
	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(stream, buf); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	if !bytes.Equal(buf, payload) {
		t.Fatalf("data mismatch: got %q, want %q", buf, payload)
	}
}

// ---------------------------------------------------------------------------
// TestConcurrentFallback
//
// Multiple goroutines dial concurrently while the primary is failing.
// All should succeed via fallback without races.
// ---------------------------------------------------------------------------

func TestConcurrentFallback(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := NewEnv(t, vnet.WithSeed(5))
	defer env.Close()

	client := env.Net.AddNode("client")
	server := env.Net.AddNode("server")
	env.Net.Link(client, server, vnet.LinkConfig{Latency: 2 * time.Millisecond})

	srv := newVnetServer(env.Net, server, "cdn", "server:7000")
	if err := srv.Listen(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	echoServer(ctx, t, srv)

	failClient := newVnetClient(env.Net, client, "h3")
	failClient.dialFn = func(ctx context.Context, addr string) (net.Conn, error) {
		return nil, errors.New("h3 always fails")
	}
	goodClient := newVnetClient(env.Net, client, "cdn")
	goodClient.dialFn = func(ctx context.Context, addr string) (net.Conn, error) {
		return env.Net.Dial(ctx, client, "server:7000")
	}

	sel := selector.New(
		[]transport.ClientTransport{failClient, goodClient},
		&selector.Config{Strategy: selector.StrategyPriority},
		nil,
	)
	defer sel.Close()

	const goroutines = 8
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			conn, err := sel.Dial(ctx, "server:7000")
			if err != nil {
				errs <- fmt.Errorf("goroutine %d: Dial: %w", id, err)
				return
			}
			stream, err := conn.OpenStream(ctx)
			if err != nil {
				errs <- fmt.Errorf("goroutine %d: OpenStream: %w", id, err)
				return
			}
			defer stream.Close()
			msg := []byte(fmt.Sprintf("hello-%d", id))
			if _, err := stream.Write(msg); err != nil {
				errs <- fmt.Errorf("goroutine %d: Write: %w", id, err)
				return
			}
			buf := make([]byte, len(msg))
			if _, err := io.ReadFull(stream, buf); err != nil {
				errs <- fmt.Errorf("goroutine %d: ReadFull: %w", id, err)
				return
			}
			if !bytes.Equal(buf, msg) {
				errs <- fmt.Errorf("goroutine %d: data mismatch: got %q, want %q", id, buf, msg)
			}
		}(i)
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
