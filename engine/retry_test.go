package engine

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryWithBackoff_SuccessFirstAttempt(t *testing.T) {
	calls := 0
	err := retryWithBackoff(context.Background(), DefaultRetryConfig(), func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestRetryWithBackoff_SuccessAfterRetry(t *testing.T) {
	calls := 0
	err := retryWithBackoff(context.Background(), RetryConfig{
		MaxAttempts:    5,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		Jitter:         0,
	}, func() error {
		calls++
		if calls < 3 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestRetryWithBackoff_AllFail(t *testing.T) {
	sentinel := errors.New("persistent failure")
	calls := 0
	err := retryWithBackoff(context.Background(), RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		Jitter:         0,
	}, func() error {
		calls++
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestRetryWithBackoff_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := retryWithBackoff(ctx, RetryConfig{
		MaxAttempts:    5,
		InitialBackoff: 500 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
		Jitter:         0,
	}, func() error {
		calls++
		if calls == 1 {
			// Cancel context so the backoff sleep is interrupted
			cancel()
		}
		return errors.New("fail")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestRetryWithBackoff_ExponentialGrowth(t *testing.T) {
	cfg := RetryConfig{
		MaxAttempts:    4,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
		Jitter:         0, // no jitter for deterministic timing
	}

	start := time.Now()
	calls := 0
	_ = retryWithBackoff(context.Background(), cfg, func() error {
		calls++
		return errors.New("fail")
	})
	elapsed := time.Since(start)

	// Expected total backoff: 10ms + 20ms + 40ms = 70ms (3 sleeps between 4 attempts)
	// Allow generous margin for CI environments
	if elapsed < 50*time.Millisecond {
		t.Fatalf("elapsed %v is too short, expected at least ~70ms of backoff", elapsed)
	}
	if calls != 4 {
		t.Fatalf("expected 4 calls, got %d", calls)
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()
	if cfg.MaxAttempts != 3 {
		t.Fatalf("expected MaxAttempts=3, got %d", cfg.MaxAttempts)
	}
	if cfg.InitialBackoff != 100*time.Millisecond {
		t.Fatalf("expected InitialBackoff=100ms, got %v", cfg.InitialBackoff)
	}
	if cfg.MaxBackoff != 5*time.Second {
		t.Fatalf("expected MaxBackoff=5s, got %v", cfg.MaxBackoff)
	}
	if cfg.Jitter != 0.2 {
		t.Fatalf("expected Jitter=0.2, got %v", cfg.Jitter)
	}
}
