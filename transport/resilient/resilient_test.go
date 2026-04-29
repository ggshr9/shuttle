package resilient

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ggshr9/shuttle/transport"
)

// ---------- mocks ----------

type mockAddr struct{ s string }

func (a mockAddr) Network() string { return "mock" }
func (a mockAddr) String() string  { return a.s }

type mockStream struct{ id uint64 }

func (s *mockStream) Read(p []byte) (int, error)  { return 0, nil }
func (s *mockStream) Write(p []byte) (int, error) { return len(p), nil }
func (s *mockStream) Close() error                { return nil }
func (s *mockStream) StreamID() uint64             { return s.id }

type mockConn struct {
	openStreamFn func(ctx context.Context) (transport.Stream, error)
	closed       atomic.Bool
}

func newHealthyConn() *mockConn {
	return &mockConn{
		openStreamFn: func(ctx context.Context) (transport.Stream, error) {
			return &mockStream{id: 1}, nil
		},
	}
}

func newFailingConn(err error) *mockConn {
	return &mockConn{
		openStreamFn: func(ctx context.Context) (transport.Stream, error) {
			return nil, err
		},
	}
}

func (c *mockConn) OpenStream(ctx context.Context) (transport.Stream, error) {
	return c.openStreamFn(ctx)
}
func (c *mockConn) AcceptStream(ctx context.Context) (transport.Stream, error) {
	return nil, errors.New("not implemented")
}
func (c *mockConn) Close() error {
	c.closed.Store(true)
	return nil
}
func (c *mockConn) LocalAddr() net.Addr  { return mockAddr{"local"} }
func (c *mockConn) RemoteAddr() net.Addr { return mockAddr{"remote"} }

// ---------- tests ----------

func TestOpenStreamHealthy(t *testing.T) {
	conn := newHealthyConn()
	dialCalls := 0
	dial := func(ctx context.Context) (transport.Connection, error) {
		dialCalls++
		return newHealthyConn(), nil
	}

	rc := Wrap(conn, dial, Config{})
	stream, err := rc.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if stream == nil {
		t.Fatal("expected non-nil stream")
	}
	if dialCalls != 0 {
		t.Fatalf("expected no dial calls, got %d", dialCalls)
	}
}

func TestReconnectOnFailure(t *testing.T) {
	failing := newFailingConn(io.EOF)
	healthy := newHealthyConn()

	var reconnected atomic.Bool
	dial := func(ctx context.Context) (transport.Connection, error) {
		return healthy, nil
	}

	rc := Wrap(failing, dial, Config{
		OnReconnect: func() { reconnected.Store(true) },
	})
	// Skip real sleeps.
	rc.sleepFn = func(d time.Duration) {}

	stream, err := rc.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("expected no error after reconnect, got %v", err)
	}
	if stream == nil {
		t.Fatal("expected non-nil stream after reconnect")
	}
	if !reconnected.Load() {
		t.Fatal("expected OnReconnect callback to be called")
	}
	if !failing.closed.Load() {
		t.Fatal("expected old connection to be closed after reconnect")
	}
}

func TestExponentialBackoffTiming(t *testing.T) {
	failing := newFailingConn(io.EOF)
	attempt := 0

	dial := func(ctx context.Context) (transport.Connection, error) {
		attempt++
		if attempt < 4 {
			return nil, errors.New("still failing")
		}
		return newHealthyConn(), nil
	}

	rc := Wrap(failing, dial, Config{
		MaxRetries: 5,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   1 * time.Second,
	})

	var delays []time.Duration
	var mu sync.Mutex
	rc.sleepFn = func(d time.Duration) {
		mu.Lock()
		delays = append(delays, d)
		mu.Unlock()
	}

	_, err := rc.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}

	// Attempts: 0 (no sleep), 1 (sleep ~100ms), 2 (sleep ~200ms), 3 (sleep ~400ms) → success
	// So we expect 3 sleeps with delays approximately: 100ms, 200ms, 400ms (±25% jitter)
	mu.Lock()
	defer mu.Unlock()

	if len(delays) != 3 {
		t.Fatalf("expected 3 backoff sleeps, got %d: %v", len(delays), delays)
	}

	baseDurations := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond}
	for i, base := range baseDurations {
		lo := time.Duration(float64(base) * 0.74) // slightly below 75% to account for float precision
		hi := time.Duration(float64(base) * 1.26)
		if delays[i] < lo || delays[i] > hi {
			t.Errorf("delay[%d] = %v, want between %v and %v (base %v ±25%%)", i, delays[i], lo, hi, base)
		}
	}
}

func TestMaxRetriesExceeded(t *testing.T) {
	failing := newFailingConn(io.EOF)
	dialErr := errors.New("dial failed")

	dial := func(ctx context.Context) (transport.Connection, error) {
		return nil, dialErr
	}

	rc := Wrap(failing, dial, Config{MaxRetries: 3})
	rc.sleepFn = func(d time.Duration) {}

	_, err := rc.OpenStream(context.Background())
	if err == nil {
		t.Fatal("expected error when max retries exceeded")
	}
	if !errors.Is(err, dialErr) {
		t.Fatalf("expected dial error, got %v", err)
	}
}

func TestConcurrentOpenStreamSingleReconnect(t *testing.T) {
	failing := newFailingConn(io.EOF)
	var dialCount atomic.Int32

	dial := func(ctx context.Context) (transport.Connection, error) {
		dialCount.Add(1)
		// Small delay to let concurrent goroutines pile up on reconnectMu.
		time.Sleep(10 * time.Millisecond)
		return newHealthyConn(), nil
	}

	rc := Wrap(failing, dial, Config{})
	rc.sleepFn = func(d time.Duration) {}

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = rc.OpenStream(context.Background())
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d got error: %v", i, err)
		}
	}

	// Only one dial should have occurred because the reconnectMu serialises
	// reconnects and the second-onwards goroutines see a fresh connection.
	if count := dialCount.Load(); count != 1 {
		t.Fatalf("expected exactly 1 dial call, got %d", count)
	}
}

func TestOldConnectionClosedAfterReconnect(t *testing.T) {
	old := newFailingConn(io.EOF)
	replacement := newHealthyConn()

	dial := func(ctx context.Context) (transport.Connection, error) {
		return replacement, nil
	}

	rc := Wrap(old, dial, Config{})
	rc.sleepFn = func(d time.Duration) {}

	_, err := rc.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !old.closed.Load() {
		t.Fatal("expected old connection to be closed")
	}
	if replacement.closed.Load() {
		t.Fatal("replacement connection should not be closed")
	}
}

func TestNonConnectionErrorNotRetried(t *testing.T) {
	appErr := errors.New("application-level error")
	conn := newFailingConn(appErr)
	dialCalls := 0

	dial := func(ctx context.Context) (transport.Connection, error) {
		dialCalls++
		return newHealthyConn(), nil
	}

	rc := Wrap(conn, dial, Config{})

	_, err := rc.OpenStream(context.Background())
	if !errors.Is(err, appErr) {
		t.Fatalf("expected application error, got %v", err)
	}
	if dialCalls != 0 {
		t.Fatalf("expected no dial calls for non-connection error, got %d", dialCalls)
	}
}

func TestMaxDelayCap(t *testing.T) {
	failing := newFailingConn(io.EOF)
	attempt := 0

	dial := func(ctx context.Context) (transport.Connection, error) {
		attempt++
		if attempt < 6 {
			return nil, errors.New("still failing")
		}
		return newHealthyConn(), nil
	}

	rc := Wrap(failing, dial, Config{
		MaxRetries: 7,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   300 * time.Millisecond,
	})

	var delays []time.Duration
	var mu sync.Mutex
	rc.sleepFn = func(d time.Duration) {
		mu.Lock()
		delays = append(delays, d)
		mu.Unlock()
	}

	_, err := rc.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// All delays after the cap should be MaxDelay (300ms).
	for _, d := range delays {
		if d > 300*time.Millisecond {
			t.Errorf("delay %v exceeds max delay 300ms", d)
		}
	}
}
