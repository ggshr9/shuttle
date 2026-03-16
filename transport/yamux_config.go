package transport

import (
	"io"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/shuttleX/shuttle/config"
)

// YamuxSessionConfig builds a *yamux.Config from the user-facing YamuxConfig.
// If cfg is nil every field keeps its yamux default.
func YamuxSessionConfig(cfg *config.YamuxConfig) *yamux.Config {
	c := yamux.DefaultConfig()
	if cfg == nil {
		c.LogOutput = io.Discard
		return c
	}
	if cfg.MaxStreamWindowSize > 0 {
		c.MaxStreamWindowSize = cfg.MaxStreamWindowSize
	}
	if cfg.KeepAliveInterval > 0 {
		c.KeepAliveInterval = time.Duration(cfg.KeepAliveInterval) * time.Second
	}
	if cfg.ConnectionWriteTimeout > 0 {
		c.ConnectionWriteTimeout = time.Duration(cfg.ConnectionWriteTimeout) * time.Second
	}
	c.LogOutput = io.Discard // suppress yamux internal logs
	return c
}
