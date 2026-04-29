package transport

import (
	"crypto/tls"
	"fmt"
	"time"

	shuttlecrypto "github.com/ggshr9/shuttle/crypto"
)

// LoadOrGenerateCert returns the TLS certificate the server should
// present. If certFile and keyFile are both non-empty the named PEM
// pair is loaded; otherwise a fresh self-signed cert valid for 1 year
// is generated for zero-config setups. The boolean second return is
// true when the result was generated (vs. loaded), so callers can log
// the appropriate "using auto-generated certificate" notice.
//
// Hoisted from the duplicate copies that previously lived in each of
// h3/server.go, cdn/server.go, and reality/server.go.
func LoadOrGenerateCert(certFile, keyFile string) (tls.Certificate, bool, error) {
	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return tls.Certificate{}, false, fmt.Errorf("load tls cert: %w", err)
		}
		return cert, false, nil
	}
	certPEM, keyPEM, err := shuttlecrypto.GenerateSelfSignedCert(nil, 365*24*time.Hour)
	if err != nil {
		return tls.Certificate{}, false, fmt.Errorf("generate self-signed cert: %w", err)
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, false, fmt.Errorf("parse self-signed cert: %w", err)
	}
	return cert, true, nil
}
