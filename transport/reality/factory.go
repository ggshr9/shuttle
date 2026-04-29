package reality

import (
	"log/slog"

	"github.com/shuttleX/shuttle/adapter"
	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/transport"
)

func init() {
	adapter.Register(&factory{})
}

type factory struct{}

func (f *factory) Type() string { return "reality" }

func (f *factory) NewClient(cfg *config.ClientConfig, opts adapter.FactoryOptions) (adapter.ClientTransport, error) {
	if !cfg.Transport.Reality.Enabled {
		return nil, nil
	}
	rCfg := &ClientConfig{
		ServerAddr:  cfg.Server.Addr,
		ServerName:  cfg.Transport.Reality.ServerName,
		ShortID:     cfg.Transport.Reality.ShortID,
		PublicKey:   cfg.Transport.Reality.PublicKey,
		Password:    cfg.Server.Password,
		PostQuantum: cfg.Transport.Reality.PostQuantum,
		Yamux:       &cfg.Yamux,
	}
	cli := NewClient(rCfg)
	if hm, ok := opts.HandshakeMetrics.(*transport.HandshakeMetrics); ok && hm != nil {
		cli.SetHandshakeMetrics(hm)
	}
	return cli, nil
}

func (f *factory) NewServer(cfg *config.ServerConfig, opts adapter.FactoryOptions) (adapter.ServerTransport, error) {
	if !cfg.Transport.Reality.Enabled {
		return nil, nil
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	sCfg := &ServerConfig{
		ListenAddr:  cfg.Listen,
		PrivateKey:  cfg.Auth.PrivateKey,
		ShortIDs:    cfg.Transport.Reality.ShortIDs,
		TargetSNI:   cfg.Transport.Reality.TargetSNI,
		TargetAddr:  cfg.Transport.Reality.TargetAddr,
		CertFile:    cfg.TLS.CertFile,
		KeyFile:     cfg.TLS.KeyFile,
		PostQuantum: cfg.Transport.Reality.PostQuantum,
		Yamux:       &cfg.Yamux,
	}
	srv, err := NewServer(sCfg, logger)
	if err != nil {
		return nil, err
	}
	if hm, ok := opts.HandshakeMetrics.(*transport.HandshakeMetrics); ok && hm != nil {
		srv.SetHandshakeMetrics(hm)
	}
	return srv, nil
}
