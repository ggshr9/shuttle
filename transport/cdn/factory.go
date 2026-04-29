package cdn

import (
	"log/slog"

	"github.com/shuttleX/shuttle/adapter"
	"github.com/shuttleX/shuttle/config"
	"github.com/shuttleX/shuttle/transport"
)

func init() {
	adapter.Register(&h2Factory{})
	adapter.Register(&grpcFactory{})
}

// h2Factory handles CDN HTTP/2 client transport.
type h2Factory struct{}

func (f *h2Factory) Type() string { return "cdn-h2" }

func (f *h2Factory) NewClient(cfg *config.ClientConfig, opts adapter.FactoryOptions) (adapter.ClientTransport, error) {
	if !cfg.Transport.CDN.Enabled || cfg.Transport.CDN.Mode == "grpc" {
		return nil, nil
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	h2Cfg := &H2Config{
		ServerAddr:         cfg.Server.Addr,
		CDNDomain:          cfg.Transport.CDN.Domain,
		Path:               cfg.Transport.CDN.Path,
		Host:               cfg.Transport.CDN.Domain,
		Password:           cfg.Server.Password,
		FrontDomain:        cfg.Transport.CDN.FrontDomain,
		InsecureSkipVerify: cfg.Transport.CDN.InsecureSkipVerify,
	}
	cli := NewH2Client(h2Cfg, WithH2Logger(logger))
	if hm, ok := opts.HandshakeMetrics.(*transport.HandshakeMetrics); ok && hm != nil {
		cli.SetHandshakeMetrics(hm)
	}
	return cli, nil
}

func (f *h2Factory) NewServer(cfg *config.ServerConfig, opts adapter.FactoryOptions) (adapter.ServerTransport, error) {
	if !cfg.Transport.CDN.Enabled {
		return nil, nil
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	sCfg := &ServerConfig{
		ListenAddr: cfg.Transport.CDN.Listen,
		CertFile:   cfg.TLS.CertFile,
		KeyFile:    cfg.TLS.KeyFile,
		Password:   cfg.Auth.Password,
		Path:       cfg.Transport.CDN.Path,
	}
	if sCfg.ListenAddr == "" {
		sCfg.ListenAddr = cfg.Listen
	}
	srv := NewServer(sCfg, logger)
	if hm, ok := opts.HandshakeMetrics.(*transport.HandshakeMetrics); ok && hm != nil {
		srv.SetHandshakeMetrics(hm)
	}
	return srv, nil
}

// grpcFactory handles CDN gRPC client transport.
type grpcFactory struct{}

func (f *grpcFactory) Type() string { return "cdn-grpc" }

func (f *grpcFactory) NewClient(cfg *config.ClientConfig, opts adapter.FactoryOptions) (adapter.ClientTransport, error) {
	if !cfg.Transport.CDN.Enabled || cfg.Transport.CDN.Mode != "grpc" {
		return nil, nil
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	grpcCfg := &GRPCConfig{
		ServerAddr:  cfg.Server.Addr,
		CDNDomain:   cfg.Transport.CDN.Domain,
		Password:    cfg.Server.Password,
		Host:        cfg.Transport.CDN.Domain,
		FrontDomain: cfg.Transport.CDN.FrontDomain,
	}
	cli := NewGRPCClient(grpcCfg, WithGRPCLogger(logger))
	if hm, ok := opts.HandshakeMetrics.(*transport.HandshakeMetrics); ok && hm != nil {
		cli.SetHandshakeMetrics(hm)
	}
	return cli, nil
}

func (f *grpcFactory) NewServer(cfg *config.ServerConfig, opts adapter.FactoryOptions) (adapter.ServerTransport, error) {
	// CDN server handles both H2 and gRPC — the h2Factory's NewServer covers this.
	return nil, nil
}
