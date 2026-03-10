package crypto

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestCertExpiry(t *testing.T) {
	validFor := 24 * time.Hour
	before := time.Now().Add(validFor)
	certPEM, _, err := GenerateSelfSignedCert([]string{"localhost"}, validFor)
	if err != nil {
		t.Fatalf("generate cert: %v", err)
	}
	after := time.Now().Add(validFor)

	expiry, err := CertExpiry(certPEM)
	if err != nil {
		t.Fatalf("CertExpiry: %v", err)
	}

	// The expiry should be between before and after (within the generation window).
	if expiry.Before(before.Add(-time.Second)) || expiry.After(after.Add(time.Second)) {
		t.Errorf("expiry %v not in expected range [%v, %v]", expiry, before, after)
	}
}

func TestCertExpiry_InvalidPEM(t *testing.T) {
	_, err := CertExpiry([]byte("not a cert"))
	if err == nil {
		t.Fatal("expected error for invalid PEM")
	}
}

func TestCertWatcherNoRenewalNeeded(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")

	// Generate a cert valid for 30 days — well outside the 7-day renewal window.
	certPEM, keyPEM, err := GenerateSelfSignedCert([]string{"localhost"}, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("generate cert: %v", err)
	}
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	w := NewCertWatcher(&CertWatcherConfig{
		CertFile:    certFile,
		KeyFile:     keyFile,
		Hosts:       []string{"localhost"},
		ValidFor:    30 * 24 * time.Hour,
		RenewBefore: 7 * 24 * time.Hour,
	}, slog.Default())

	renewed, err := w.checkAndRenew()
	if err != nil {
		t.Fatalf("checkAndRenew: %v", err)
	}
	if renewed {
		t.Error("expected no renewal for cert with 30 days remaining")
	}
}

func TestCertWatcherRenewsExpiring(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")

	// Generate a cert valid for only 1 hour — well within the 7-day renewal window.
	certPEM, keyPEM, err := GenerateSelfSignedCert([]string{"localhost"}, 1*time.Hour)
	if err != nil {
		t.Fatalf("generate cert: %v", err)
	}
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	w := NewCertWatcher(&CertWatcherConfig{
		CertFile:    certFile,
		KeyFile:     keyFile,
		Hosts:       []string{"localhost"},
		ValidFor:    30 * 24 * time.Hour,
		RenewBefore: 7 * 24 * time.Hour,
	}, slog.Default())

	renewed, err := w.checkAndRenew()
	if err != nil {
		t.Fatalf("checkAndRenew: %v", err)
	}
	if !renewed {
		t.Fatal("expected renewal for cert expiring in 1 hour")
	}

	// Verify the new cert has a longer validity.
	newCertPEM, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatalf("read renewed cert: %v", err)
	}
	expiry, err := CertExpiry(newCertPEM)
	if err != nil {
		t.Fatalf("parse renewed cert: %v", err)
	}
	remaining := time.Until(expiry)
	if remaining < 29*24*time.Hour {
		t.Errorf("renewed cert should be valid for ~30 days, got %v", remaining)
	}
}

func TestCertWatcherCallback(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")

	// Cert that expires in 1 hour — triggers renewal.
	certPEM, keyPEM, err := GenerateSelfSignedCert([]string{"localhost"}, 1*time.Hour)
	if err != nil {
		t.Fatalf("generate cert: %v", err)
	}
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	var called atomic.Bool
	w := NewCertWatcher(&CertWatcherConfig{
		CertFile:    certFile,
		KeyFile:     keyFile,
		Hosts:       []string{"localhost"},
		ValidFor:    30 * 24 * time.Hour,
		RenewBefore: 7 * 24 * time.Hour,
		OnRenew:     func() { called.Store(true) },
	}, slog.Default())

	renewed, err := w.checkAndRenew()
	if err != nil {
		t.Fatalf("checkAndRenew: %v", err)
	}
	if !renewed {
		t.Fatal("expected renewal")
	}
	if !called.Load() {
		t.Error("onRenew callback was not called")
	}
}

func TestCertWatcherStartStop(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")

	// Cert valid for 30 days — no renewal expected.
	certPEM, keyPEM, err := GenerateSelfSignedCert([]string{"localhost"}, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("generate cert: %v", err)
	}
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	w := NewCertWatcher(&CertWatcherConfig{
		CertFile:      certFile,
		KeyFile:       keyFile,
		Hosts:         []string{"localhost"},
		ValidFor:      30 * 24 * time.Hour,
		CheckInterval: time.Hour, // won't fire in this test
	}, slog.Default())

	w.Start(context.Background())
	w.Stop() // Should return promptly without deadlock.
}

func TestCertWatcherDefaults(t *testing.T) {
	w := NewCertWatcher(&CertWatcherConfig{}, nil)
	if w.renewBefore != 7*24*time.Hour {
		t.Errorf("default renewBefore = %v, want 7 days", w.renewBefore)
	}
	if w.checkInterval != 12*time.Hour {
		t.Errorf("default checkInterval = %v, want 12 hours", w.checkInterval)
	}
	if w.logger == nil {
		t.Error("logger should not be nil when nil is passed")
	}
}
