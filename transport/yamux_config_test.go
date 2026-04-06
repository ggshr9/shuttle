package transport

import (
	"testing"
	"time"

	"github.com/shuttleX/shuttle/config"
)

func TestYamuxSessionConfig_Nil(t *testing.T) {
	cfg := YamuxSessionConfig(nil)
	if cfg == nil {
		t.Fatal("expected non-nil config for nil input")
		return
	}
	// Should return default values.
	if cfg.MaxStreamWindowSize != 256*1024 {
		t.Errorf("MaxStreamWindowSize = %d, want %d", cfg.MaxStreamWindowSize, 256*1024)
	}
}

func TestYamuxSessionConfig_Custom(t *testing.T) {
	ycfg := &config.YamuxConfig{
		MaxStreamWindowSize:    512 * 1024,
		KeepAliveInterval:      60,
		ConnectionWriteTimeout: 20,
	}
	cfg := YamuxSessionConfig(ycfg)
	if cfg.MaxStreamWindowSize != 512*1024 {
		t.Errorf("MaxStreamWindowSize = %d, want %d", cfg.MaxStreamWindowSize, 512*1024)
	}
	if cfg.KeepAliveInterval != 60*time.Second {
		t.Errorf("KeepAliveInterval = %v, want 60s", cfg.KeepAliveInterval)
	}
	if cfg.ConnectionWriteTimeout != 20*time.Second {
		t.Errorf("ConnectionWriteTimeout = %v, want 20s", cfg.ConnectionWriteTimeout)
	}
}

func TestYamuxSessionConfig_ZeroValuesKeepDefaults(t *testing.T) {
	ycfg := &config.YamuxConfig{} // all zeros
	cfg := YamuxSessionConfig(ycfg)
	// With all-zero input, defaults should be preserved.
	if cfg.MaxStreamWindowSize != 256*1024 {
		t.Errorf("MaxStreamWindowSize = %d, want default %d", cfg.MaxStreamWindowSize, 256*1024)
	}
	if cfg.KeepAliveInterval != 30*time.Second {
		t.Errorf("KeepAliveInterval = %v, want default 30s", cfg.KeepAliveInterval)
	}
	if cfg.ConnectionWriteTimeout != 10*time.Second {
		t.Errorf("ConnectionWriteTimeout = %v, want default 10s", cfg.ConnectionWriteTimeout)
	}
}

func TestYamuxSessionConfig_PartialOverride(t *testing.T) {
	ycfg := &config.YamuxConfig{
		MaxStreamWindowSize: 1024 * 1024, // 1 MB
		// KeepAliveInterval and ConnectionWriteTimeout left at 0 (keep defaults)
	}
	cfg := YamuxSessionConfig(ycfg)
	if cfg.MaxStreamWindowSize != 1024*1024 {
		t.Errorf("MaxStreamWindowSize = %d, want %d", cfg.MaxStreamWindowSize, 1024*1024)
	}
	if cfg.KeepAliveInterval != 30*time.Second {
		t.Errorf("KeepAliveInterval = %v, want default 30s", cfg.KeepAliveInterval)
	}
}
