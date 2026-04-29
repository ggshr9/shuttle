package webrtc

import (
	"log/slog"

	"github.com/ggshr9/shuttle/adapter"
	"github.com/ggshr9/shuttle/config"
)

func init() {
	adapter.Register(&factory{})
}

type factory struct{}

func (f *factory) Type() string { return "webrtc" }

func (f *factory) NewClient(cfg *config.ClientConfig, opts adapter.FactoryOptions) (adapter.ClientTransport, error) {
	if !cfg.Transport.WebRTC.Enabled {
		return nil, nil
	}
	wCfg := &ClientConfig{
		SignalURL:   cfg.Transport.WebRTC.SignalURL,
		Password:    cfg.Server.Password,
		STUNServers: cfg.Transport.WebRTC.STUNServers,
		TURNServers: cfg.Transport.WebRTC.TURNServers,
		TURNUser:    cfg.Transport.WebRTC.TURNUser,
		TURNPass:    cfg.Transport.WebRTC.TURNPass,
		ICEPolicy:   cfg.Transport.WebRTC.ICEPolicy,
	}
	return NewClient(wCfg), nil
}

func (f *factory) NewServer(cfg *config.ServerConfig, opts adapter.FactoryOptions) (adapter.ServerTransport, error) {
	if !cfg.Transport.WebRTC.Enabled {
		return nil, nil
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	sCfg := &ServerConfig{
		SignalListen: cfg.Transport.WebRTC.SignalListen,
		CertFile:     cfg.TLS.CertFile,
		KeyFile:      cfg.TLS.KeyFile,
		Password:     cfg.Auth.Password,
		STUNServers:  cfg.Transport.WebRTC.STUNServers,
		TURNServers:  cfg.Transport.WebRTC.TURNServers,
		TURNUser:     cfg.Transport.WebRTC.TURNUser,
		TURNPass:     cfg.Transport.WebRTC.TURNPass,
		ICEPolicy:    cfg.Transport.WebRTC.ICEPolicy,
	}
	return NewServer(sCfg, logger), nil
}
