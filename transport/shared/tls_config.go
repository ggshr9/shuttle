package shared

import (
	"crypto/tls"
	"fmt"
)

// TLSOptions holds client-side TLS configuration.
type TLSOptions struct {
	Enabled            bool     `json:"enabled" yaml:"enabled"`
	ServerName         string   `json:"server_name" yaml:"server_name"`
	InsecureSkipVerify bool     `json:"insecure_skip_verify,omitempty" yaml:"insecure_skip_verify,omitempty"`
	ALPN               []string `json:"alpn,omitempty" yaml:"alpn,omitempty"`
}

// ServerTLSOptions holds server-side TLS configuration.
type ServerTLSOptions struct {
	CertFile string   `json:"cert_file" yaml:"cert_file"`
	KeyFile  string   `json:"key_file" yaml:"key_file"`
	ALPN     []string `json:"alpn,omitempty" yaml:"alpn,omitempty"`
}

// BuildClientTLS constructs a *tls.Config from the given TLSOptions.
// Returns nil, nil if opts.Enabled is false.
func BuildClientTLS(opts TLSOptions) (*tls.Config, error) {
	if !opts.Enabled {
		return nil, nil
	}
	cfg := &tls.Config{
		ServerName:         opts.ServerName,
		InsecureSkipVerify: opts.InsecureSkipVerify, //nolint:gosec // intentional opt-in
		NextProtos:         opts.ALPN,
	}
	return cfg, nil
}

// BuildServerTLS constructs a *tls.Config from the given ServerTLSOptions.
// It loads the X.509 key pair from CertFile and KeyFile.
func BuildServerTLS(opts ServerTLSOptions) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(opts.CertFile, opts.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("shared/tls: load key pair: %w", err)
	}
	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   opts.ALPN,
	}
	return cfg, nil
}
