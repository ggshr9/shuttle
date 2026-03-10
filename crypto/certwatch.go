package crypto

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

// CertWatcherConfig holds configuration for CertWatcher.
type CertWatcherConfig struct {
	// CertFile is the path to the TLS certificate PEM file.
	CertFile string
	// KeyFile is the path to the TLS private key PEM file.
	KeyFile string
	// Hosts are the hostnames/IPs for certificate regeneration.
	Hosts []string
	// ValidFor is the validity duration for regenerated certificates.
	ValidFor time.Duration
	// RenewBefore is how far before expiry to trigger renewal (default 7 days).
	RenewBefore time.Duration
	// CheckInterval is how often to check certificate expiry (default 12 hours).
	CheckInterval time.Duration
	// OnRenew is called after a certificate is successfully renewed.
	OnRenew func()
}

// CertWatcher monitors a TLS certificate file and auto-regenerates it
// when it is about to expire.
type CertWatcher struct {
	certFile      string
	keyFile       string
	hosts         []string
	validFor      time.Duration
	renewBefore   time.Duration
	checkInterval time.Duration
	logger        *slog.Logger
	onRenew       func()

	cancel context.CancelFunc
	ctx    context.Context
	wg     sync.WaitGroup
}

// NewCertWatcher creates a new CertWatcher from the given config.
func NewCertWatcher(cfg CertWatcherConfig, logger *slog.Logger) *CertWatcher {
	if cfg.RenewBefore == 0 {
		cfg.RenewBefore = 7 * 24 * time.Hour
	}
	if cfg.CheckInterval == 0 {
		cfg.CheckInterval = 12 * time.Hour
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &CertWatcher{
		certFile:      cfg.CertFile,
		keyFile:       cfg.KeyFile,
		hosts:         cfg.Hosts,
		validFor:      cfg.ValidFor,
		renewBefore:   cfg.RenewBefore,
		checkInterval: cfg.CheckInterval,
		logger:        logger,
		onRenew:       cfg.OnRenew,
	}
}

// Start begins the certificate watch loop in a background goroutine.
// It immediately checks the certificate and then rechecks on the configured interval.
func (w *CertWatcher) Start(ctx context.Context) {
	w.ctx, w.cancel = context.WithCancel(ctx)
	w.wg.Add(1)
	go w.run()
}

// Stop cancels the watch loop and waits for the goroutine to exit.
func (w *CertWatcher) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
}

func (w *CertWatcher) run() {
	defer w.wg.Done()

	// Immediate check on start.
	if renewed, err := w.checkAndRenew(); err != nil {
		w.logger.Error("cert watch: initial check failed", "error", err)
	} else if renewed {
		w.logger.Info("cert watch: certificate renewed on startup")
	}

	ticker := time.NewTicker(w.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			if renewed, err := w.checkAndRenew(); err != nil {
				w.logger.Error("cert watch: check failed", "error", err)
			} else if renewed {
				w.logger.Info("cert watch: certificate renewed")
			}
		}
	}
}

// checkAndRenew reads the certificate file, checks if it expires within
// the renewBefore window, and regenerates it if necessary.
func (w *CertWatcher) checkAndRenew() (renewed bool, err error) {
	certPEM, err := os.ReadFile(w.certFile)
	if err != nil {
		return false, fmt.Errorf("read cert file: %w", err)
	}

	expiry, err := CertExpiry(certPEM)
	if err != nil {
		return false, fmt.Errorf("parse cert expiry: %w", err)
	}

	remaining := time.Until(expiry)
	if remaining > w.renewBefore {
		w.logger.Debug("cert watch: certificate still valid",
			"expires", expiry,
			"remaining", remaining.Round(time.Second),
		)
		return false, nil
	}

	w.logger.Info("cert watch: certificate expiring soon, renewing",
		"expires", expiry,
		"remaining", remaining.Round(time.Second),
	)

	newCert, newKey, err := GenerateSelfSignedCert(w.hosts, w.validFor)
	if err != nil {
		return false, fmt.Errorf("generate cert: %w", err)
	}

	if err := os.WriteFile(w.certFile, newCert, 0o600); err != nil {
		return false, fmt.Errorf("write cert file: %w", err)
	}
	if err := os.WriteFile(w.keyFile, newKey, 0o600); err != nil {
		return false, fmt.Errorf("write key file: %w", err)
	}

	if w.onRenew != nil {
		w.onRenew()
	}

	return true, nil
}
