package vnet

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/ggshr9/shuttle/testkit/observe"
)

func TestBasicDialListen(t *testing.T) {
	net := New(WithSeed(1))
	defer net.Close()

	a := net.AddNode("a")
	b := net.AddNode("b")
	net.Link(a, b, LinkConfig{})

	ln, err := net.Listen(b, ":8080")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// Accept in background.
	done := make(chan struct{})
	go func() {
		defer close(done)
		c, err := ln.Accept()
		if err != nil {
			return
		}
		buf := make([]byte, 64)
		n, _ := c.Read(buf)
		c.Write(buf[:n])
	}()

	conn, err := net.Dial(context.Background(), a, ":8080")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	msg := []byte("hello vnet")
	if _, err := conn.Write(msg); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "hello vnet" {
		t.Fatalf("got %q, want %q", buf[:n], "hello vnet")
	}
}

func TestLatency(t *testing.T) {
	net := New(WithSeed(2))
	defer net.Close()

	a := net.AddNode("a")
	b := net.AddNode("b")
	net.Link(a, b, LinkConfig{Latency: 50 * time.Millisecond})

	ln, err := net.Listen(b, ":80")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		buf := make([]byte, 64)
		n, _ := c.Read(buf)
		c.Write(buf[:n])
	}()

	conn, err := net.Dial(context.Background(), a, ":80")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	start := time.Now()
	conn.Write([]byte("ping"))
	buf := make([]byte, 64)
	conn.Read(buf)
	elapsed := time.Since(start)

	// Round trip = 2 * 50ms = 100ms. Allow 50ms–500ms.
	if elapsed < 50*time.Millisecond {
		t.Fatalf("too fast: %v (expected >= 50ms)", elapsed)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("too slow: %v (expected < 500ms)", elapsed)
	}
}

func TestLoss(t *testing.T) {
	net := New(WithSeed(42))
	defer net.Close()

	a := net.AddNode("a")
	b := net.AddNode("b")
	net.Link(a, b, LinkConfig{Loss: 0.5})

	ln, err := net.Listen(b, ":80")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	const total = 200

	// Server: read and count what arrives.
	received := make(chan int, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			received <- 0
			return
		}
		count := 0
		buf := make([]byte, 16)
		for {
			_, err := c.Read(buf)
			if err != nil {
				break
			}
			count++
		}
		received <- count
	}()

	conn, err := net.Dial(context.Background(), a, ":80")
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < total; i++ {
		conn.Write([]byte("x"))
	}
	conn.Close()

	got := <-received
	ratio := float64(got) / float64(total)
	// With 50% loss and 200 writes, expect ~100 received. Allow 20%-80%.
	if ratio < 0.2 || ratio > 0.8 {
		t.Fatalf("loss ratio out of range: got %d/%d = %.2f", got, total, ratio)
	}
}

func TestBandwidth(t *testing.T) {
	net := New(WithSeed(3))
	defer net.Close()

	a := net.AddNode("a")
	b := net.AddNode("b")
	net.Link(a, b, LinkConfig{Bandwidth: 1024}) // 1KB/s

	ln, err := net.Listen(b, ":80")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// Server: read everything and signal when done.
	serverDone := make(chan int64, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			serverDone <- 0
			return
		}
		n, _ := io.Copy(io.Discard, c)
		serverDone <- n
	}()

	conn, err := net.Dial(context.Background(), a, ":80")
	if err != nil {
		t.Fatal(err)
	}

	// Measure time for 1KB to be received at the other end.
	data := make([]byte, 1024) // 1KB
	start := time.Now()
	conn.Write(data)
	conn.Close()

	<-serverDone
	elapsed := time.Since(start)

	// 1KB at 1KB/s should take ~1s. Allow 0.5s–3s.
	if elapsed < 500*time.Millisecond {
		t.Fatalf("too fast: %v (expected ~1s for 1KB at 1KB/s)", elapsed)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("too slow: %v", elapsed)
	}
}

func TestJitter(t *testing.T) {
	net := New(WithSeed(4))
	defer net.Close()

	a := net.AddNode("a")
	b := net.AddNode("b")
	net.Link(a, b, LinkConfig{
		Latency: 50 * time.Millisecond,
		Jitter:  20 * time.Millisecond,
	})

	ln, err := net.Listen(b, ":80")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		// Echo each message individually.
		buf := make([]byte, 16)
		for {
			n, err := c.Read(buf)
			if err != nil {
				return
			}
			c.Write(buf[:n])
		}
	}()

	conn, err := net.Dial(context.Background(), a, ":80")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	var durations []time.Duration
	for i := 0; i < 10; i++ {
		start := time.Now()
		conn.Write([]byte("p"))
		buf := make([]byte, 16)
		conn.Read(buf)
		durations = append(durations, time.Since(start))
	}

	// Check that not all durations are identical (jitter should vary them).
	allSame := true
	for i := 1; i < len(durations); i++ {
		diff := durations[i] - durations[0]
		if diff < 0 {
			diff = -diff
		}
		if diff > 2*time.Millisecond {
			allSame = false
			break
		}
	}
	if allSame {
		t.Fatal("all round-trip times are identical; jitter not applied")
	}
}

func TestDeterministic(t *testing.T) {
	runTrial := func(seed int64) []bool {
		net := New(WithSeed(seed))
		defer net.Close()

		a := net.AddNode("a")
		b := net.AddNode("b")
		net.Link(a, b, LinkConfig{Loss: 0.5})

		ln, _ := net.Listen(b, ":80")
		defer ln.Close()

		results := make(chan []bool, 1)
		go func() {
			c, err := ln.Accept()
			if err != nil {
				results <- nil
				return
			}
			var got []bool
			buf := make([]byte, 16)
			for {
				_, err := c.Read(buf)
				if err != nil {
					break
				}
				got = append(got, true)
			}
			results <- got
		}()

		conn, _ := net.Dial(context.Background(), a, ":80")
		pattern := make([]bool, 50)
		for range pattern {
			conn.Write([]byte("x"))
		}
		conn.Close()

		received := <-results
		// Mark which writes were received.
		for i := range received {
			if i < len(pattern) {
				pattern[i] = true
			}
		}
		return pattern
	}

	// Two runs with the same seed should have the same number of received messages.
	r1 := runTrial(99)
	r2 := runTrial(99)

	count := func(b []bool) int {
		n := 0
		for _, v := range b {
			if v {
				n++
			}
		}
		return n
	}

	if count(r1) != count(r2) {
		t.Fatalf("non-deterministic: run1 received %d, run2 received %d", count(r1), count(r2))
	}
}

func TestMultiNode(t *testing.T) {
	net := New(WithSeed(5))
	defer net.Close()

	a := net.AddNode("a")
	b := net.AddNode("b")
	c := net.AddNode("c")

	net.Link(a, b, LinkConfig{})
	net.Link(b, c, LinkConfig{})

	// Listen on C.
	ln, err := net.Listen(c, ":80")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		buf := make([]byte, 64)
		n, _ := conn.Read(buf)
		conn.Write(buf[:n])
	}()

	// Dial from A to C (multi-hop via B).
	conn, err := net.Dial(context.Background(), a, ":80")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	conn.Write([]byte("multi-hop"))
	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "multi-hop" {
		t.Fatalf("got %q, want %q", buf[:n], "multi-hop")
	}
}

func TestConcurrent(t *testing.T) {
	net := New(WithSeed(6))
	defer net.Close()

	server := net.AddNode("server")
	ln, _ := net.Listen(server, ":80")
	defer ln.Close()

	// Server: echo each connection.
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				buf := make([]byte, 256)
				for {
					n, err := c.Read(buf)
					if err != nil {
						return
					}
					c.Write(buf[:n])
				}
			}()
		}
	}()

	const numClients = 10
	var wg sync.WaitGroup

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			client := net.AddNode("")
			net.Link(client, server, LinkConfig{})

			conn, err := net.Dial(context.Background(), client, ":80")
			if err != nil {
				t.Errorf("client %d: dial: %v", id, err)
				return
			}
			defer conn.Close()

			msg := []byte("hello")
			conn.Write(msg)
			buf := make([]byte, 64)
			n, err := conn.Read(buf)
			if err != nil {
				t.Errorf("client %d: read: %v", id, err)
				return
			}
			if string(buf[:n]) != "hello" {
				t.Errorf("client %d: got %q", id, buf[:n])
			}
		}(i)
	}
	wg.Wait()
}

func TestAsymmetricLink(t *testing.T) {
	net := New(WithSeed(7))
	defer net.Close()

	a := net.AddNode("a")
	b := net.AddNode("b")
	// A->B: 100ms latency, B->A: 10ms latency
	net.LinkAsymmetric(a, b,
		LinkConfig{Latency: 100 * time.Millisecond},
		LinkConfig{Latency: 10 * time.Millisecond},
	)

	ln, _ := net.Listen(b, ":80")
	defer ln.Close()

	// Server: immediately reply.
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		buf := make([]byte, 64)
		n, _ := c.Read(buf)
		c.Write(buf[:n])
	}()

	conn, _ := net.Dial(context.Background(), a, ":80")
	defer conn.Close()

	start := time.Now()
	conn.Write([]byte("x"))
	buf := make([]byte, 64)
	conn.Read(buf)
	elapsed := time.Since(start)

	// RTT should be ~110ms (100ms + 10ms). Allow 80ms–500ms.
	if elapsed < 80*time.Millisecond {
		t.Fatalf("too fast: %v", elapsed)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("too slow: %v", elapsed)
	}
}

func TestDialUnknownAddr(t *testing.T) {
	net := New(WithSeed(8))
	defer net.Close()

	a := net.AddNode("a")
	_, err := net.Dial(context.Background(), a, ":9999")
	if err == nil {
		t.Fatal("expected error dialing unknown address")
	}
}

func TestListenerClose(t *testing.T) {
	net := New(WithSeed(9))
	defer net.Close()

	a := net.AddNode("a")
	b := net.AddNode("b")
	net.Link(a, b, LinkConfig{})

	ln, _ := net.Listen(b, ":80")
	ln.Close()

	_, err := net.Dial(context.Background(), a, ":80")
	if err == nil {
		t.Fatal("expected error dialing closed listener")
	}
}

func TestConnClose(t *testing.T) {
	net := New(WithSeed(10))
	defer net.Close()

	a := net.AddNode("a")
	b := net.AddNode("b")
	net.Link(a, b, LinkConfig{})

	ln, _ := net.Listen(b, ":80")
	defer ln.Close()

	done := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		buf := make([]byte, 64)
		_, err = c.Read(buf)
		done <- err // should be EOF or io.ErrClosedPipe
	}()

	conn, _ := net.Dial(context.Background(), a, ":80")
	conn.Close()

	err := <-done
	if err == nil {
		t.Fatal("expected error after conn close")
	}
}

func TestZeroConfig(t *testing.T) {
	net := New(WithSeed(11))
	defer net.Close()

	a := net.AddNode("a")
	b := net.AddNode("b")
	net.Link(a, b, LinkConfig{}) // zero config = no conditions

	ln, _ := net.Listen(b, ":80")
	defer ln.Close()

	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		buf := make([]byte, 64)
		n, _ := c.Read(buf)
		c.Write(buf[:n])
	}()

	conn, _ := net.Dial(context.Background(), a, ":80")
	defer conn.Close()

	start := time.Now()
	conn.Write([]byte("fast"))
	buf := make([]byte, 64)
	n, _ := conn.Read(buf)
	elapsed := time.Since(start)

	if string(buf[:n]) != "fast" {
		t.Fatalf("got %q", buf[:n])
	}
	// Should be near-instant. Allow up to 50ms.
	if elapsed > 50*time.Millisecond {
		t.Fatalf("zero config too slow: %v", elapsed)
	}
}

func TestDynamicLinkUpdate(t *testing.T) {
	rec := observe.NewRecorderManual()
	net := New(WithSeed(20), WithRecorder(rec))
	defer net.Close()

	a := net.AddNode("a")
	b := net.AddNode("b")
	net.Link(a, b, LinkConfig{Latency: 5 * time.Millisecond})

	ln, err := net.Listen(b, ":80")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		buf := make([]byte, 256)
		for {
			n, err := c.Read(buf)
			if err != nil {
				return
			}
			c.Write(buf[:n])
		}
	}()

	conn, err := net.Dial(context.Background(), a, ":80")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Phase 1: Fast link (5ms latency).
	start := time.Now()
	conn.Write([]byte("fast"))
	buf := make([]byte, 64)
	conn.Read(buf)
	fast := time.Since(start)

	// Phase 2: Degrade link to 100ms latency.
	net.UpdateLink(a, b, LinkConfig{Latency: 100 * time.Millisecond})
	net.UpdateLink(b, a, LinkConfig{Latency: 100 * time.Millisecond})

	start = time.Now()
	conn.Write([]byte("slow"))
	conn.Read(buf)
	slow := time.Since(start)

	// The slow round-trip should be significantly longer.
	if slow < 100*time.Millisecond {
		t.Fatalf("after degradation, RTT = %v, expected >= 100ms", slow)
	}
	if fast > slow {
		t.Fatalf("fast RTT (%v) should be less than slow RTT (%v)", fast, slow)
	}

	// Verify link-update events were recorded.
	updates := rec.Filter("link-update")
	if len(updates) < 2 {
		t.Fatalf("expected at least 2 link-update events, got %d", len(updates))
	}
}

func TestMultiHopLatencyAccumulation(t *testing.T) {
	net := New(WithSeed(21))
	defer net.Close()

	a := net.AddNode("a")
	b := net.AddNode("b")
	c := net.AddNode("c")

	// a->b: 30ms, b->c: 40ms. Multi-hop a->c should accumulate to ~70ms.
	net.Link(a, b, LinkConfig{Latency: 30 * time.Millisecond})
	net.Link(b, c, LinkConfig{Latency: 40 * time.Millisecond})

	ln, err := net.Listen(c, ":80")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		buf := make([]byte, 64)
		n, _ := conn.Read(buf)
		conn.Write(buf[:n])
	}()

	conn, err := net.Dial(context.Background(), a, ":80")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	start := time.Now()
	conn.Write([]byte("hop"))
	buf := make([]byte, 64)
	conn.Read(buf)
	rtt := time.Since(start)

	// Expected RTT = (30+40)*2 = 140ms. Allow 100ms-500ms.
	if rtt < 100*time.Millisecond {
		t.Fatalf("multi-hop RTT too fast: %v (expected >= 100ms)", rtt)
	}
	if rtt > 500*time.Millisecond {
		t.Fatalf("multi-hop RTT too slow: %v", rtt)
	}
}

func TestReorderConfig(t *testing.T) {
	rec := observe.NewRecorderManual()
	net := New(WithSeed(30), WithRecorder(rec))
	defer net.Close()

	a := net.AddNode("a")
	b := net.AddNode("b")
	net.Link(a, b, LinkConfig{
		Latency:      5 * time.Millisecond,
		ReorderPct:   0.5,
		ReorderDelay: 20 * time.Millisecond,
	})

	ln, err := net.Listen(b, ":80")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		io.Copy(io.Discard, c)
	}()

	conn, err := net.Dial(context.Background(), a, ":80")
	if err != nil {
		t.Fatal(err)
	}

	// Send enough messages that some should be reordered.
	for i := 0; i < 50; i++ {
		conn.Write([]byte("x"))
	}
	conn.Close()

	// Verify reorder events were recorded.
	reorders := rec.Filter("reorder")
	if len(reorders) == 0 {
		t.Fatal("expected reorder events with 50% reorder probability")
	}
	t.Logf("reorder events: %d out of 50 writes", len(reorders))
}

func TestRecorderAssertions(t *testing.T) {
	rec := observe.NewRecorderManual()
	rec.Record(observe.Event{Kind: "dial", From: "a", To: "b"})
	rec.Record(observe.Event{Kind: "drop", From: "link", Size: 100})
	rec.Record(observe.Event{Kind: "drop", From: "link", Size: 200})
	rec.Record(observe.Event{Kind: "send", From: "a", To: "b", Size: 500})

	// Count
	if c := rec.Count("drop"); c != 2 {
		t.Fatalf("Count(drop) = %d, want 2", c)
	}
	if c := rec.Count("dial"); c != 1 {
		t.Fatalf("Count(dial) = %d, want 1", c)
	}
	if c := rec.Count("nonexistent"); c != 0 {
		t.Fatalf("Count(nonexistent) = %d, want 0", c)
	}

	// Filter
	drops := rec.Filter("drop")
	if len(drops) != 2 {
		t.Fatalf("Filter(drop) len = %d, want 2", len(drops))
	}

	// FilterFrom
	linkDrops := rec.FilterFrom("drop", "link")
	if len(linkDrops) != 2 {
		t.Fatalf("FilterFrom(drop, link) len = %d, want 2", len(linkDrops))
	}
	aDrops := rec.FilterFrom("drop", "a")
	if len(aDrops) != 0 {
		t.Fatalf("FilterFrom(drop, a) len = %d, want 0", len(aDrops))
	}

	// TotalBytes
	if total := rec.TotalBytes("drop"); total != 300 {
		t.Fatalf("TotalBytes(drop) = %d, want 300", total)
	}
}
