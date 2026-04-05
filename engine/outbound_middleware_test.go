package engine

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

// failOutbound is a test helper that fails N times then succeeds.
type failOutbound struct {
	tag       string
	typ       string
	failCount int
	calls     atomic.Int32
}

func (f *failOutbound) Tag() string  { return f.tag }
func (f *failOutbound) Type() string { return f.typ }
func (f *failOutbound) Close() error { return nil }

func (f *failOutbound) DialContext(_ context.Context, _, _ string) (net.Conn, error) {
	n := int(f.calls.Add(1))
	if n <= f.failCount {
		return nil, errors.New("dial failed")
	}
	// Return one end of a pipe as a successful connection
	c1, _ := net.Pipe()
	return c1, nil
}

func TestResilientOutbound_RetrySuccess(t *testing.T) {
	inner := &failOutbound{tag: "test", typ: "mock", failCount: 2}
	ro := NewResilientOutbound(inner, ResilientOutboundConfig{
		RetryConfig: RetryConfig{
			MaxAttempts:    3,
			InitialBackoff: 1 * time.Millisecond,
			MaxBackoff:     10 * time.Millisecond,
			Jitter:         0,
		},
	})

	conn, err := ro.DialContext(context.Background(), "tcp", "127.0.0.1:80")
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	conn.Close()

	if got := int(inner.calls.Load()); got != 3 {
		t.Fatalf("expected 3 total attempts, got %d", got)
	}
}

func TestResilientOutbound_CircuitBreaker(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Threshold:    3,
		BaseCooldown: 1 * time.Hour, // long cooldown so it stays open
	})

	// Inner always fails
	inner := &failOutbound{tag: "cb-test", typ: "mock", failCount: 1000}
	ro := NewResilientOutbound(inner, ResilientOutboundConfig{
		CircuitBreaker: cb,
		RetryConfig: RetryConfig{
			MaxAttempts:    1, // no retries, just 1 attempt per call
			InitialBackoff: 1 * time.Millisecond,
		},
	})

	// Make enough calls to trip the circuit breaker
	for i := 0; i < 3; i++ {
		_, err := ro.DialContext(context.Background(), "tcp", "127.0.0.1:80")
		if err == nil {
			t.Fatal("expected error")
		}
	}

	if cb.State() != CircuitOpen {
		t.Fatalf("expected circuit open, got %s", cb.State())
	}

	// Next call should get circuit breaker error
	_, err := ro.DialContext(context.Background(), "tcp", "127.0.0.1:80")
	if err == nil {
		t.Fatal("expected circuit breaker error")
	}
	if got := err.Error(); got != `circuit breaker open for outbound "cb-test"` {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestResilientOutbound_PreservesTagAndType(t *testing.T) {
	inner := &failOutbound{tag: "my-tag", typ: "my-type", failCount: 0}
	ro := NewResilientOutbound(inner, ResilientOutboundConfig{})

	if ro.Tag() != "my-tag" {
		t.Fatalf("Tag() = %q, want %q", ro.Tag(), "my-tag")
	}
	if ro.Type() != "my-type" {
		t.Fatalf("Type() = %q, want %q", ro.Type(), "my-type")
	}
}

func TestResilientOutbound_NilCircuitBreaker(t *testing.T) {
	inner := &failOutbound{tag: "nil-cb", typ: "mock", failCount: 1}
	ro := NewResilientOutbound(inner, ResilientOutboundConfig{
		CircuitBreaker: nil,
		RetryConfig: RetryConfig{
			MaxAttempts:    2,
			InitialBackoff: 1 * time.Millisecond,
			MaxBackoff:     10 * time.Millisecond,
			Jitter:         0,
		},
	})

	conn, err := ro.DialContext(context.Background(), "tcp", "127.0.0.1:80")
	if err != nil {
		t.Fatalf("expected success with nil CB, got: %v", err)
	}
	conn.Close()

	if got := int(inner.calls.Load()); got != 2 {
		t.Fatalf("expected 2 attempts, got %d", got)
	}
}
