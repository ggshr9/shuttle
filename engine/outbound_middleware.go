package engine

import (
	"context"
	"fmt"
	"net"

	"github.com/shuttleX/shuttle/adapter"
)

// Compile-time check that ResilientOutbound implements adapter.Outbound.
var _ adapter.Outbound = (*ResilientOutbound)(nil)

// ResilientOutboundConfig configures the resilience middleware.
type ResilientOutboundConfig struct {
	CircuitBreaker *CircuitBreaker
	RetryConfig    RetryConfig
}

// ResilientOutbound wraps an adapter.Outbound with circuit breaker and retry.
type ResilientOutbound struct {
	inner adapter.Outbound
	cb    *CircuitBreaker
	retry RetryConfig
}

// NewResilientOutbound creates a resilient outbound wrapper.
// If RetryConfig.MaxAttempts == 0, DefaultRetryConfig() is used.
// CircuitBreaker can be nil (CB checks are skipped).
func NewResilientOutbound(inner adapter.Outbound, cfg ResilientOutboundConfig) *ResilientOutbound {
	rc := cfg.RetryConfig
	if rc.MaxAttempts == 0 {
		rc = DefaultRetryConfig()
	}
	return &ResilientOutbound{
		inner: inner,
		cb:    cfg.CircuitBreaker,
		retry: rc,
	}
}

// Tag delegates to the inner outbound.
func (r *ResilientOutbound) Tag() string { return r.inner.Tag() }

// Type delegates to the inner outbound.
func (r *ResilientOutbound) Type() string { return r.inner.Type() }

// Close delegates to the inner outbound.
func (r *ResilientOutbound) Close() error { return r.inner.Close() }

// DialContext dials through the inner outbound with circuit breaker and retry.
func (r *ResilientOutbound) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	// Check circuit breaker before attempting
	if r.cb != nil && !r.cb.Allow() {
		return nil, fmt.Errorf("circuit breaker open for outbound %q", r.inner.Tag())
	}

	var conn net.Conn
	err := retryWithBackoff(ctx, r.retry, func() error {
		var dialErr error
		conn, dialErr = r.inner.DialContext(ctx, network, address)
		return dialErr
	})

	if r.cb != nil {
		if err != nil {
			r.cb.RecordFailure()
		} else {
			r.cb.RecordSuccess()
		}
	}

	return conn, err
}
