package fault

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/shuttleX/shuttle/testkit/vnet"
	"github.com/shuttleX/shuttle/transport"
)

// --- helpers ---

// pipeStream wraps one side of a net.Pipe as a transport.Stream.
type pipeStream struct {
	conn net.Conn
	id   uint64
}

var _ transport.Stream = (*pipeStream)(nil)

func (s *pipeStream) Read(b []byte) (int, error)  { return s.conn.Read(b) }
func (s *pipeStream) Write(b []byte) (int, error) { return s.conn.Write(b) }
func (s *pipeStream) Close() error                { return s.conn.Close() }
func (s *pipeStream) StreamID() uint64            { return s.id }

func newStreamPair(id uint64) (transport.Stream, transport.Stream) {
	a, b := net.Pipe()
	return &pipeStream{conn: a, id: id}, &pipeStream{conn: b, id: id}
}

// --- tests ---

func TestNoRulesPassthrough(t *testing.T) {
	fi := New()
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	wrapped := fi.WrapConn(a)

	msg := []byte("hello world")
	go func() {
		_, _ = wrapped.Write(msg)
	}()

	buf := make([]byte, 64)
	n, err := b.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf[:n], msg) {
		t.Fatalf("got %q, want %q", buf[:n], msg)
	}
}

func TestDelayOnRead(t *testing.T) {
	fi := New()
	fi.OnRead().Delay(100 * time.Millisecond).Install()

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	wrapped := fi.WrapConn(a)

	go func() {
		_, _ = b.Write([]byte("data"))
	}()

	buf := make([]byte, 64)
	start := time.Now()
	n, err := wrapped.Read(buf)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Fatalf("got %d bytes, want 4", n)
	}
	if elapsed < 90*time.Millisecond {
		t.Fatalf("read took %v, expected >= 100ms", elapsed)
	}
}

func TestDelayOnWrite(t *testing.T) {
	fi := New()
	fi.OnWrite().Delay(100 * time.Millisecond).Install()

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	wrapped := fi.WrapConn(a)

	// Reader goroutine to unblock the pipe.
	go func() {
		buf := make([]byte, 64)
		_, _ = b.Read(buf)
	}()

	start := time.Now()
	_, err := wrapped.Write([]byte("data"))
	elapsed := time.Since(start)

	if err != nil {
		t.Fatal(err)
	}
	if elapsed < 90*time.Millisecond {
		t.Fatalf("write took %v, expected >= 100ms", elapsed)
	}
}

func TestErrorOnRead(t *testing.T) {
	fi := New()
	errFake := errors.New("injected read error")
	fi.OnRead().Error(errFake).Install()

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	wrapped := fi.WrapConn(a)

	go func() {
		_, _ = b.Write([]byte("data"))
	}()

	buf := make([]byte, 64)
	_, err := wrapped.Read(buf)
	if !errors.Is(err, errFake) {
		t.Fatalf("got err=%v, want %v", err, errFake)
	}
}

func TestErrorOnWrite(t *testing.T) {
	fi := New()
	errFake := errors.New("injected write error")
	fi.OnWrite().Error(errFake).Install()

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	_ = b // keep alive

	wrapped := fi.WrapConn(a)
	_, err := wrapped.Write([]byte("data"))
	if !errors.Is(err, errFake) {
		t.Fatalf("got err=%v, want %v", err, errFake)
	}
}

func TestDropOnWrite(t *testing.T) {
	fi := New()
	fi.OnWrite().Drop().Install()

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	wrapped := fi.WrapConn(a)

	n, err := wrapped.Write([]byte("dropped"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 7 {
		t.Fatalf("got n=%d, want 7", n)
	}

	// Nothing should arrive at b. Use a deadline to check.
	_ = b.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	buf := make([]byte, 64)
	_, err = b.Read(buf)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestCorrupt(t *testing.T) {
	fi := New()
	fi.OnWrite().Corrupt(42).Install()

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	wrapped := fi.WrapConn(a)

	// Use a large-enough payload so corruption is detectable.
	original := bytes.Repeat([]byte("AAAA"), 64)

	go func() {
		_, _ = wrapped.Write(original)
	}()

	buf := make([]byte, len(original))
	n, err := io.ReadFull(b, buf)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(buf[:n], original) {
		t.Fatal("data was not corrupted")
	}
}

func TestProbability(t *testing.T) {
	errFault := errors.New("prob error")

	// Use a single injector so the RNG sequence progresses across calls.
	fi := New().WithSeed(42)
	fi.OnWrite().Error(errFault).WithProbability(0.5).Install()

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	// Drain reader so non-error writes don't block.
	go func() {
		buf := make([]byte, 64)
		for {
			if _, err := b.Read(buf); err != nil {
				return
			}
		}
	}()

	wrapped := fi.WrapConn(a)

	hits := 0
	total := 200
	for i := 0; i < total; i++ {
		_, err := wrapped.Write([]byte("x"))
		if errors.Is(err, errFault) {
			hits++
		}
	}

	ratio := float64(hits) / float64(total)
	// Accept a wide range: 15%-85%. We just need to show it's not always 0 or always 1.
	if ratio < 0.15 || ratio > 0.85 {
		t.Fatalf("probability way off: %d/%d = %.2f, expected ~0.5", hits, total, ratio)
	}
}

func TestAfterDuration(t *testing.T) {
	fi := New()
	errFault := errors.New("after error")
	fi.OnWrite().Error(errFault).After(100 * time.Millisecond).Install()

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	wrapped := fi.WrapConn(a)

	// Immediate write should pass through.
	go func() {
		buf := make([]byte, 64)
		_, _ = b.Read(buf)
	}()
	_, err := wrapped.Write([]byte("early"))
	if err != nil {
		t.Fatalf("expected no error before 'after' duration, got %v", err)
	}

	// Wait past the 'after' threshold.
	time.Sleep(120 * time.Millisecond)

	_, err = wrapped.Write([]byte("late"))
	if !errors.Is(err, errFault) {
		t.Fatalf("got err=%v, want %v", err, errFault)
	}
}

func TestTimesLimit(t *testing.T) {
	fi := New()
	errFault := errors.New("times error")
	fi.OnWrite().Error(errFault).Times(2).Install()

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	wrapped := fi.WrapConn(a)

	// First two writes should fail.
	for i := 0; i < 2; i++ {
		_, err := wrapped.Write([]byte("x"))
		if !errors.Is(err, errFault) {
			t.Fatalf("write %d: got err=%v, want %v", i, err, errFault)
		}
	}

	// Third write should pass through.
	go func() {
		buf := make([]byte, 64)
		_, _ = b.Read(buf)
	}()
	_, err := wrapped.Write([]byte("ok"))
	if err != nil {
		t.Fatalf("write 3: expected nil error, got %v", err)
	}
}

func TestMultipleRules(t *testing.T) {
	fi := New()
	err1 := errors.New("first rule")
	err2 := errors.New("second rule")
	fi.OnWrite().Error(err1).Times(1).Install()
	fi.OnWrite().Error(err2).Install()

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	wrapped := fi.WrapConn(a)

	// First write: first rule matches.
	_, err := wrapped.Write([]byte("x"))
	if !errors.Is(err, err1) {
		t.Fatalf("got err=%v, want %v", err, err1)
	}

	// Second write: first rule exhausted, second rule matches.
	_, err = wrapped.Write([]byte("x"))
	if !errors.Is(err, err2) {
		t.Fatalf("got err=%v, want %v", err, err2)
	}
}

func TestWrapStream(t *testing.T) {
	fi := New()
	errFault := errors.New("stream error")
	fi.OnRead().Error(errFault).Install()

	sA, sB := newStreamPair(99)
	defer sA.Close()
	defer sB.Close()

	wrapped := fi.WrapStream(sA)

	// Verify StreamID is delegated.
	if wrapped.StreamID() != 99 {
		t.Fatalf("got StreamID=%d, want 99", wrapped.StreamID())
	}

	go func() {
		_, _ = sB.Write([]byte("data"))
	}()

	buf := make([]byte, 64)
	_, err := wrapped.Read(buf)
	if !errors.Is(err, errFault) {
		t.Fatalf("got err=%v, want %v", err, errFault)
	}
}

func TestConcurrentAccess(t *testing.T) {
	fi := New()
	fi.OnWrite().Delay(1 * time.Millisecond).Install()
	fi.OnRead().Delay(1 * time.Millisecond).Install()

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	wrapped := fi.WrapConn(a)

	var wg sync.WaitGroup
	const goroutines = 10
	const iterations = 20

	// Writers.
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, _ = wrapped.Write([]byte("x"))
			}
		}()
	}

	// Reader to drain the pipe.
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 1024)
		for {
			_, err := b.Read(buf)
			if err != nil {
				close(done)
				return
			}
		}
	}()

	wg.Wait()
	a.Close()
	<-done
}

func TestClearRules(t *testing.T) {
	fi := New()
	errFault := errors.New("clear-me")
	fi.OnWrite().Error(errFault).Install()

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	wrapped := fi.WrapConn(a)

	// Write should fail.
	_, err := wrapped.Write([]byte("x"))
	if !errors.Is(err, errFault) {
		t.Fatalf("before clear: got err=%v, want %v", err, errFault)
	}

	// Clear write rules.
	fi.ClearWrite()

	// Write should now pass.
	go func() {
		buf := make([]byte, 64)
		_, _ = b.Read(buf)
	}()
	_, err = wrapped.Write([]byte("ok"))
	if err != nil {
		t.Fatalf("after clear: got err=%v, want nil", err)
	}
}

func TestClearAllRules(t *testing.T) {
	fi := New()
	errR := errors.New("read-err")
	errW := errors.New("write-err")
	fi.OnRead().Error(errR).Install()
	fi.OnWrite().Error(errW).Install()

	fi.ClearRules()

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	wrapped := fi.WrapConn(a)

	// Both should pass through.
	go func() {
		_, _ = wrapped.Write([]byte("data"))
	}()
	buf := make([]byte, 64)
	n, err := b.Read(buf)
	if err != nil {
		t.Fatalf("write passthrough: err=%v", err)
	}
	if !bytes.Equal(buf[:n], []byte("data")) {
		t.Fatalf("got %q, want %q", buf[:n], "data")
	}
}

func TestWithSeed(t *testing.T) {
	// Verify that WithSeed produces deterministic results.
	results := make([]int, 2)
	for trial := 0; trial < 2; trial++ {
		hits := 0
		for i := 0; i < 100; i++ {
			fi := New().WithSeed(42 + int64(i))
			fi.OnWrite().Error(errors.New("x")).WithProbability(0.5).Install()
			a, b := net.Pipe()
			go func() {
				buf := make([]byte, 64)
				for {
					if _, err := b.Read(buf); err != nil {
						return
					}
				}
			}()
			wrapped := fi.WrapConn(a)
			_, err := wrapped.Write([]byte("x"))
			if err != nil {
				hits++
			}
			a.Close()
			b.Close()
		}
		results[trial] = hits
	}
	if results[0] != results[1] {
		t.Fatalf("WithSeed not deterministic: trial1=%d, trial2=%d", results[0], results[1])
	}
}

func TestChainedBuilder(t *testing.T) {
	fi := New()
	errFault := errors.New("chained error")

	// Build a rule using the full fluent chain.
	fi.OnRead().
		Error(errFault).
		WithProbability(1.0).
		After(0).
		Times(1).
		Install()

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	wrapped := fi.WrapConn(a)

	go func() {
		_, _ = b.Write([]byte("data"))
	}()

	buf := make([]byte, 64)
	_, err := wrapped.Read(buf)
	if !errors.Is(err, errFault) {
		t.Fatalf("got err=%v, want %v", err, errFault)
	}

	// Second read should pass through (Times=1 exhausted).
	go func() {
		_, _ = b.Write([]byte("more"))
	}()
	n, err := wrapped.Read(buf)
	if err != nil {
		t.Fatalf("second read: unexpected err=%v", err)
	}
	if !bytes.Equal(buf[:n], []byte("more")) {
		t.Fatalf("got %q, want %q", buf[:n], "more")
	}
}

func TestVirtualClockAfter(t *testing.T) {
	// Verify that fault injection with VirtualClock works deterministically
	// without wall-clock sleeps.
	vc := vnet.NewVirtualClock(time.Time{})
	fi := New().WithClock(vc)

	errFault := errors.New("virtual after error")
	fi.OnWrite().Error(errFault).After(500 * time.Millisecond).Install()

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	wrapped := fi.WrapConn(a)

	// Before the "After" threshold — should pass through.
	go func() {
		buf := make([]byte, 64)
		_, _ = b.Read(buf)
	}()
	_, err := wrapped.Write([]byte("early"))
	if err != nil {
		t.Fatalf("expected no error before 'after' duration, got %v", err)
	}

	// Advance virtual clock past the threshold.
	vc.Advance(600 * time.Millisecond)

	_, err = wrapped.Write([]byte("late"))
	if !errors.Is(err, errFault) {
		t.Fatalf("got err=%v, want %v", err, errFault)
	}
}

func TestVirtualClockDelay(t *testing.T) {
	// Verify that Delay action uses VirtualClock, not wall-clock time.
	vc := vnet.NewVirtualClock(time.Time{})
	fi := New().WithClock(vc)

	fi.OnRead().Delay(1 * time.Second).Install()

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	wrapped := fi.WrapConn(a)

	go func() {
		_, _ = b.Write([]byte("data"))
	}()

	// Start read in a goroutine — it should block on virtual delay
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 64)
		_, err := wrapped.Read(buf)
		done <- err
	}()

	// Wait for the virtual clock waiter to be registered, then advance
	vc.BlockUntilWaiters(1)
	vc.Advance(2 * time.Second)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("read blocked on wall-clock — VirtualClock not working")
	}
}
