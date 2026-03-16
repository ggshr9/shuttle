package scenarios

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/shuttleX/shuttle/testkit/vnet"
)

// ---------------------------------------------------------------------------
// TestDynamicDegradationDuringTransfer
//
// Establishes a connection over a clean link, starts streaming data,
// then degrades the link mid-transfer (increases latency + adds loss).
// Verifies that data continues to flow despite degradation.
// ---------------------------------------------------------------------------

func TestDynamicDegradationDuringTransfer(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := NewEnv(t, vnet.WithSeed(100))
	defer env.Close()

	client := env.Net.AddNode("client")
	server := env.Net.AddNode("server")
	env.Net.Link(client, server, vnet.LinkConfig{Latency: 5 * time.Millisecond})

	srv := newVnetServer(env.Net, server, "h3", "server:9000")
	if err := srv.Listen(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	echoServer(ctx, t, srv)

	clientTransport := newVnetClient(env.Net, client, "h3")
	clientTransport.dialFn = func(ctx context.Context, addr string) (net.Conn, error) {
		return env.Net.Dial(ctx, client, "server:9000")
	}

	conn, err := clientTransport.Dial(ctx, "server:9000")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	// Phase 1: Clean link — verify fast echo.
	stream1, err := conn.OpenStream(ctx)
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}

	msg1 := []byte("before-degradation")
	start := time.Now()
	if _, err := stream1.Write(msg1); err != nil {
		t.Fatalf("Write: %v", err)
	}
	buf1 := make([]byte, len(msg1))
	if _, err := io.ReadFull(stream1, buf1); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	fastRTT := time.Since(start)
	stream1.Close()

	if !bytes.Equal(buf1, msg1) {
		t.Fatalf("msg1 mismatch: got %q, want %q", buf1, msg1)
	}

	// Phase 2: Degrade the link.
	env.Net.UpdateLink(client, server, vnet.LinkConfig{
		Latency: 100 * time.Millisecond,
		Loss:    0.1,
	})
	env.Net.UpdateLink(server, client, vnet.LinkConfig{
		Latency: 100 * time.Millisecond,
		Loss:    0.1,
	})

	// Phase 3: Verify data still flows (may be slower).
	stream2, err := conn.OpenStream(ctx)
	if err != nil {
		t.Fatalf("OpenStream after degradation: %v", err)
	}
	defer stream2.Close()

	msg2 := []byte("after-degradation")
	start = time.Now()
	if _, err := stream2.Write(msg2); err != nil {
		t.Fatalf("Write: %v", err)
	}
	buf2 := make([]byte, len(msg2))
	if _, err := io.ReadFull(stream2, buf2); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	slowRTT := time.Since(start)

	if !bytes.Equal(buf2, msg2) {
		t.Fatalf("msg2 mismatch: got %q, want %q", buf2, msg2)
	}

	t.Logf("fast RTT: %v, slow RTT: %v", fastRTT, slowRTT)

	// Verify the degradation is visible in the recorder.
	env.Recorder.AssertHas(t, "link-update")

	// Verify link-update events.
	updates := env.Recorder.Filter("link-update")
	if len(updates) < 2 {
		t.Fatalf("expected at least 2 link-update events, got %d", len(updates))
	}
}

// ---------------------------------------------------------------------------
// TestDynamicRecoveryAfterDegradation
//
// Link degrades then recovers. Verify latency decreases after recovery.
// ---------------------------------------------------------------------------

func TestDynamicRecoveryAfterDegradation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := NewEnv(t, vnet.WithSeed(101))
	defer env.Close()

	client := env.Net.AddNode("client")
	server := env.Net.AddNode("server")
	env.Net.Link(client, server, vnet.LinkConfig{Latency: 5 * time.Millisecond})

	srv := newVnetServer(env.Net, server, "h3", "server:9100")
	if err := srv.Listen(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	echoServer(ctx, t, srv)

	clientTransport := newVnetClient(env.Net, client, "h3")
	clientTransport.dialFn = func(ctx context.Context, addr string) (net.Conn, error) {
		return env.Net.Dial(ctx, client, "server:9100")
	}

	conn, err := clientTransport.Dial(ctx, "server:9100")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	roundTrip := func(msg string) time.Duration {
		stream, err := conn.OpenStream(ctx)
		if err != nil {
			t.Fatalf("OpenStream: %v", err)
		}
		defer stream.Close()
		payload := []byte(msg)
		start := time.Now()
		stream.Write(payload)
		buf := make([]byte, len(payload))
		io.ReadFull(stream, buf)
		return time.Since(start)
	}

	// Phase 1: Fast.
	rtt1 := roundTrip("phase-1")

	// Phase 2: Degrade.
	env.Net.UpdateLink(client, server, vnet.LinkConfig{Latency: 100 * time.Millisecond})
	env.Net.UpdateLink(server, client, vnet.LinkConfig{Latency: 100 * time.Millisecond})
	rtt2 := roundTrip("phase-2")

	// Phase 3: Recover.
	env.Net.UpdateLink(client, server, vnet.LinkConfig{Latency: 5 * time.Millisecond})
	env.Net.UpdateLink(server, client, vnet.LinkConfig{Latency: 5 * time.Millisecond})
	rtt3 := roundTrip("phase-3")

	t.Logf("RTT phase1=%v, phase2=%v, phase3=%v", rtt1, rtt2, rtt3)

	// Phase 2 should be slowest, phase 3 should recover close to phase 1.
	if rtt2 < rtt1 {
		t.Fatalf("degraded RTT (%v) should be > initial RTT (%v)", rtt2, rtt1)
	}
	if rtt3 > rtt2 {
		t.Fatalf("recovered RTT (%v) should be < degraded RTT (%v)", rtt3, rtt2)
	}
}
