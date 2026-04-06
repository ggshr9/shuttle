package crypto

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"
)

func TestGenerateSelfSignedCert(t *testing.T) {
	certPEM, keyPEM, err := GenerateSelfSignedCert([]string{"example.com", "1.2.3.4"}, 24*time.Hour)
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert() error = %v", err)
	}

	// Verify PEM encoding
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		t.Fatal("failed to decode cert PEM")
		return
	}
	if certBlock.Type != "CERTIFICATE" {
		t.Errorf("cert PEM type = %q, want CERTIFICATE", certBlock.Type)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		t.Fatal("failed to decode key PEM")
		return
	}
	if keyBlock.Type != "EC PRIVATE KEY" {
		t.Errorf("key PEM type = %q, want EC PRIVATE KEY", keyBlock.Type)
	}

	// Parse certificate
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}

	// Check SANs
	if len(cert.DNSNames) != 1 || cert.DNSNames[0] != "example.com" {
		t.Errorf("DNSNames = %v, want [example.com]", cert.DNSNames)
	}
	if len(cert.IPAddresses) != 1 || cert.IPAddresses[0].String() != "1.2.3.4" {
		t.Errorf("IPAddresses = %v, want [1.2.3.4]", cert.IPAddresses)
	}

	// Check validity period
	if cert.NotAfter.Sub(cert.NotBefore) != 24*time.Hour {
		t.Errorf("validity = %v, want 24h", cert.NotAfter.Sub(cert.NotBefore))
	}

	// Verify cert+key pair works together
	_, err = tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair() error = %v", err)
	}
}

func TestGenerateSelfSignedCertDefaults(t *testing.T) {
	// No hosts — should add localhost defaults
	certPEM, _, err := GenerateSelfSignedCert(nil, time.Hour)
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert(nil) error = %v", err)
	}

	certBlock, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}

	if len(cert.DNSNames) == 0 || cert.DNSNames[0] != "localhost" {
		t.Errorf("DNSNames = %v, want [localhost]", cert.DNSNames)
	}
	if len(cert.IPAddresses) == 0 || cert.IPAddresses[0].String() != "127.0.0.1" {
		t.Errorf("IPAddresses = %v, want [127.0.0.1]", cert.IPAddresses)
	}
}
