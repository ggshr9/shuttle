package engine

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/shuttleX/shuttle/adapter"
	"github.com/shuttleX/shuttle/plugin"
)

// Compile-time checks.
var _ adapter.Outbound = (*ResilientOutbound)(nil)
var _ adapter.Outbound = (*ChainOutbound)(nil)

// ChainOutbound wraps an adapter.Outbound so connections flow through a plugin chain.
type ChainOutbound struct {
	inner adapter.Outbound
	chain *plugin.Chain
}

// NewChainOutbound creates a ChainOutbound wrapper.
func NewChainOutbound(inner adapter.Outbound, chain *plugin.Chain) *ChainOutbound {
	return &ChainOutbound{inner: inner, chain: chain}
}

func (c *ChainOutbound) Tag() string  { return c.inner.Tag() }
func (c *ChainOutbound) Type() string { return c.inner.Type() }
func (c *ChainOutbound) Close() error { return c.inner.Close() }

func (c *ChainOutbound) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	conn, err := c.inner.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	wrapped, err := c.chain.OnConnect(conn, address)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &chainConn{Conn: wrapped, chain: c.chain}, nil
}

// ResilientOutboundConfig configures the resilience middleware.
type ResilientOutboundConfig struct {
	CircuitBreaker *CircuitBreaker
	RetryConfig    RetryConfig
	// OnRetry is called after each failed dial attempt that will be retried.
	// attempt is the 1-based attempt number that just failed.
	// Optional; nil disables the callback.
	OnRetry func(attempt int, err error)
}

// ResilientOutbound wraps an adapter.Outbound with circuit breaker and retry.
type ResilientOutbound struct {
	inner   adapter.Outbound
	cb      *CircuitBreaker
	retry   RetryConfig
	onRetry func(attempt int, err error)
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
		inner:   inner,
		cb:      cfg.CircuitBreaker,
		retry:   rc,
		onRetry: cfg.OnRetry,
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
	// Check circuit breaker before attempting.
	if r.cb != nil && !r.cb.Allow() {
		return nil, fmt.Errorf("circuit breaker open for outbound %q", r.inner.Tag())
	}

	cfg := r.retry
	maxAttempts := cfg.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	backoff := cfg.InitialBackoff
	if backoff == 0 {
		backoff = 100 * time.Millisecond
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		conn, err := r.inner.DialContext(ctx, network, address)
		if err == nil {
			if r.cb != nil {
				r.cb.RecordSuccess()
			}
			return conn, nil
		}
		lastErr = err

		// Notify caller of this failed attempt if more retries remain.
		if r.onRetry != nil && attempt < maxAttempts {
			r.onRetry(attempt, err)
		}

		// Wait before next attempt (skip after last).
		if attempt < maxAttempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
			if cfg.MaxBackoff > 0 && backoff > cfg.MaxBackoff {
				backoff = cfg.MaxBackoff
			}
		}
	}

	if r.cb != nil {
		r.cb.RecordFailure()
	}
	return nil, lastErr
}
