package h3

import (
	"log/slog"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/shuttleX/shuttle/adapter"
	"github.com/shuttleX/shuttle/config"
)

func init() {
	adapter.Register(&factory{})
}

type factory struct{}

func (f *factory) Type() string { return "h3" }

func (f *factory) NewClient(cfg *config.ClientConfig, opts adapter.FactoryOptions) (adapter.ClientTransport, error) {
	if !cfg.Transport.H3.Enabled {
		return nil, nil
	}
	h3Cfg := &ClientConfig{
		ServerAddr:         cfg.Server.Addr,
		ServerName:         cfg.Server.SNI,
		Password:           cfg.Server.Password,
		PathPrefix:         cfg.Transport.H3.PathPrefix,
		InsecureSkipVerify: cfg.Transport.H3.InsecureSkipVerify,
	}
	if cc, ok := opts.CongestionControl.(quic.CongestionControl); ok && cc != nil {
		h3Cfg.CongestionControl = cc
	}
	mp := cfg.Transport.H3.Multipath
	if mp.Enabled {
		probe := 5 * time.Second
		if mp.ProbeInterval != "" {
			if d, err := time.ParseDuration(mp.ProbeInterval); err == nil {
				probe = d
			}
		}
		h3Cfg.Multipath = &MultipathConfig{
			Enabled:       true,
			Interfaces:    mp.Interfaces,
			Mode:          mp.Mode,
			ProbeInterval: probe,
		}
	}
	return NewClient(h3Cfg), nil
}

func (f *factory) NewServer(cfg *config.ServerConfig, opts adapter.FactoryOptions) (adapter.ServerTransport, error) {
	if !cfg.Transport.H3.Enabled {
		return nil, nil
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	sCfg := &ServerConfig{
		ListenAddr: cfg.Listen,
		CertFile:   cfg.TLS.CertFile,
		KeyFile:    cfg.TLS.KeyFile,
		Password:   cfg.Auth.Password,
		PathPrefix: cfg.Transport.H3.PathPrefix,
	}
	if cc, ok := opts.CongestionControl.(quic.CongestionControl); ok && cc != nil {
		sCfg.CongestionControl = cc
	}
	return NewServer(sCfg, logger), nil
}
