package sysopt

import (
	"log/slog"
	"runtime"
	"testing"
)

func TestApply(t *testing.T) {
	logger := slog.Default()
	// Should not panic
	Apply(logger)

	// GOMAXPROCS should be set to NumCPU
	if runtime.GOMAXPROCS(0) != runtime.NumCPU() {
		t.Fatalf("GOMAXPROCS = %d, want %d", runtime.GOMAXPROCS(0), runtime.NumCPU())
	}
}

func TestApplyNilLogger(t *testing.T) {
	// Should not panic with nil logger
	Apply(nil)
}

func TestRecommendedBufferSize(t *testing.T) {
	size := RecommendedBufferSize()
	if size != 4*1024*1024 {
		t.Fatalf("RecommendedBufferSize = %d, want %d", size, 4*1024*1024)
	}
}

func TestApplyPlatformOpts(t *testing.T) {
	// Just verify it doesn't panic on current platform
	applyPlatformOpts(slog.Default())
}
