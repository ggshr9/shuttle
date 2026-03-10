package engine

import (
	"context"
	"math/rand"
	"time"
)

// RetryConfig configures connection retry behavior.
type RetryConfig struct {
	MaxAttempts    int           // max retry attempts (default 3)
	InitialBackoff time.Duration // initial backoff duration (default 100ms)
	MaxBackoff     time.Duration // max backoff duration (default 5s)
	Jitter         float64       // jitter factor 0-1 (default 0.2)
}

// DefaultRetryConfig returns sensible defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
		Jitter:         0.2,
	}
}

// retryWithBackoff executes fn with exponential backoff.
// Returns the result of the first successful call, or the last error.
func retryWithBackoff(ctx context.Context, cfg RetryConfig, fn func() error) error {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 1
	}

	var lastErr error
	backoff := cfg.InitialBackoff
	if backoff == 0 {
		backoff = 100 * time.Millisecond
	}

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		// Don't sleep after the last attempt
		if attempt < cfg.MaxAttempts-1 {
			// Add jitter
			jittered := backoff
			if cfg.Jitter > 0 {
				jitter := time.Duration(float64(backoff) * cfg.Jitter * (rand.Float64()*2 - 1)) //nolint:gosec // jitter does not need cryptographic randomness
				jittered = backoff + jitter
				if jittered < 0 {
					jittered = backoff
				}
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(jittered):
			}

			// Exponential increase
			backoff *= 2
			if cfg.MaxBackoff > 0 && backoff > cfg.MaxBackoff {
				backoff = cfg.MaxBackoff
			}
		}
	}
	return lastErr
}
